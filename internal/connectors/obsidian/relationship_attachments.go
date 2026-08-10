package obsidian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"amberdesk/pkg/connectors"
)

func (c *Connector) ListRelationshipAttachments(_ context.Context, ref connectors.DossierRef, nodeID string) ([]connectors.RelationshipAttachment, error) {
	c.attachmentMu.RLock()
	defer c.attachmentMu.RUnlock()
	return c.listRelationshipAttachments(ref, nodeID)
}

func (c *Connector) listRelationshipAttachments(ref connectors.DossierRef, nodeID string) ([]connectors.RelationshipAttachment, error) {
	if _, err := c.readRelationshipNode(ref, nodeID); err != nil {
		return nil, err
	}
	directory, err := c.attachmentNodeDirectory(ref, nodeID, false)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []connectors.RelationshipAttachment{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]connectors.RelationshipAttachment, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadata, err := c.readAttachmentMetadata(filepath.Join(directory, entry.Name(), "metadata.json"))
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", entry.Name(), err)
		}
		if metadata.NodeID == nodeID {
			result = append(result, metadata)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt < result[j].CreatedAt })
	return result, nil
}

func (c *Connector) StoreRelationshipAttachment(ctx context.Context, ref connectors.DossierRef, attachment connectors.RelationshipAttachment, content []byte) (connectors.RelationshipAttachment, error) {
	c.attachmentMu.Lock()
	defer c.attachmentMu.Unlock()
	if _, err := c.readRelationshipNode(ref, attachment.NodeID); err != nil {
		return attachment, err
	}
	if err := validateAttachmentFilename(attachment.Filename); err != nil {
		return attachment, err
	}
	if int64(len(content)) > connectors.MaxRelationshipAttachmentSize {
		return attachment, connectors.ErrAttachmentLarge
	}
	items, err := c.listRelationshipAttachments(ref, attachment.NodeID)
	if err != nil {
		return attachment, err
	}
	if len(items) >= connectors.MaxRelationshipAttachmentsPerNode {
		return attachment, connectors.ErrAttachmentLimit
	}
	if !validAttachmentID(attachment.ID) {
		return attachment, connectors.ErrInvalidFilename
	}
	directory, err := c.attachmentDirectory(ref, attachment.NodeID, attachment.ID, true)
	if err != nil {
		return attachment, err
	}
	if _, err := os.Stat(filepath.Join(directory, "metadata.json")); err == nil {
		return attachment, connectors.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return attachment, err
	}
	sum := sha256.Sum256(content)
	attachment.Size = int64(len(content))
	attachment.SHA256 = hex.EncodeToString(sum[:])
	if attachment.CreatedAt == "" {
		attachment.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := atomicWrite(filepath.Join(directory, "content.bin"), content); err != nil {
		return attachment, err
	}
	metadata, err := json.MarshalIndent(attachment, "", "  ")
	if err != nil {
		return attachment, err
	}
	if err := atomicWrite(filepath.Join(directory, "metadata.json"), append(metadata, '\n')); err != nil {
		_ = os.RemoveAll(directory)
		return attachment, err
	}
	return attachment, nil
}

func (c *Connector) ReadRelationshipAttachment(_ context.Context, ref connectors.DossierRef, nodeID, attachmentID string) (connectors.RelationshipAttachment, []byte, error) {
	c.attachmentMu.RLock()
	defer c.attachmentMu.RUnlock()
	directory, err := c.attachmentDirectory(ref, nodeID, attachmentID, false)
	if err != nil {
		return connectors.RelationshipAttachment{}, nil, err
	}
	metadata, err := c.readAttachmentMetadata(filepath.Join(directory, "metadata.json"))
	if errors.Is(err, os.ErrNotExist) {
		return metadata, nil, connectors.ErrEntityAbsent
	}
	if err != nil || metadata.NodeID != nodeID || metadata.ID != attachmentID {
		return metadata, nil, errors.Join(connectors.ErrConflict, err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "content.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return metadata, nil, connectors.ErrEntityAbsent
	}
	if err != nil {
		return metadata, nil, err
	}
	sum := sha256.Sum256(content)
	if int64(len(content)) != metadata.Size || hex.EncodeToString(sum[:]) != metadata.SHA256 {
		return metadata, nil, connectors.ErrConflict
	}
	return metadata, content, nil
}

func (c *Connector) DeleteRelationshipAttachment(_ context.Context, ref connectors.DossierRef, nodeID, attachmentID string) error {
	c.attachmentMu.Lock()
	defer c.attachmentMu.Unlock()
	directory, err := c.attachmentDirectory(ref, nodeID, attachmentID, false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(directory, "metadata.json")); errors.Is(err, os.ErrNotExist) {
		return connectors.ErrEntityAbsent
	} else if err != nil {
		return err
	}
	trash, err := c.caseDirectory(ref, true, ".trash", "Attachments", sanitize(nodeID))
	if err != nil {
		return err
	}
	target := filepath.Join(trash, time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+sanitize(attachmentID))
	if err := ensureInside(c.vaultPath, target); err != nil {
		return err
	}
	return os.Rename(directory, target)
}

func (c *Connector) trashRelationshipAttachments(ref connectors.DossierRef, nodeID string) error {
	directory, err := c.attachmentNodeDirectory(ref, nodeID, false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	trash, err := c.caseDirectory(ref, true, ".trash", "AttachmentCards")
	if err != nil {
		return err
	}
	target := filepath.Join(trash, time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+sanitize(nodeID))
	return os.Rename(directory, target)
}

func (c *Connector) attachmentNodeDirectory(ref connectors.DossierRef, nodeID string, create bool) (string, error) {
	if !validAttachmentID(nodeID) {
		return "", connectors.ErrEntityAbsent
	}
	return c.caseDirectory(ref, create, "Relations", "Attachments", sanitize(nodeID))
}

func (c *Connector) attachmentDirectory(ref connectors.DossierRef, nodeID, attachmentID string, create bool) (string, error) {
	if !validAttachmentID(attachmentID) {
		return "", connectors.ErrEntityAbsent
	}
	directory, err := c.attachmentNodeDirectory(ref, nodeID, create)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, sanitize(attachmentID))
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	if create {
		err = os.MkdirAll(path, 0o700)
	}
	return path, err
}

func (c *Connector) readAttachmentMetadata(path string) (connectors.RelationshipAttachment, error) {
	if err := ensureInside(c.vaultPath, path); err != nil {
		return connectors.RelationshipAttachment{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return connectors.RelationshipAttachment{}, err
	}
	var metadata connectors.RelationshipAttachment
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, err
	}
	if err := validateAttachmentFilename(metadata.Filename); err != nil || !validAttachmentID(metadata.ID) || !validAttachmentID(metadata.NodeID) {
		return metadata, connectors.ErrConflict
	}
	return metadata, nil
}

func validateAttachmentFilename(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) || utf8.RuneCountInString(name) > 180 {
		return connectors.ErrInvalidFilename
	}
	for _, char := range name {
		if char < 32 || char == 127 {
			return connectors.ErrInvalidFilename
		}
	}
	return nil
}

func validAttachmentID(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, char := range value {
		if !(char == '-' || char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
