package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"amberdesk/internal/casefile"
	"amberdesk/pkg/connectors"
)

type memoryAttachment struct {
	Metadata connectors.RelationshipAttachment
	Content  []byte
}

var relationshipNodeTypes = map[string]bool{"object": true, "subject": true, "organization": true, "account": true, "location": true, "infrastructure": true, "evidence": true, "fact": true}
var relationshipKinds = map[string]bool{"standard": true, "critical": true, "evidence": true}

func (h *Handler) listRelationships(w http.ResponseWriter, r *http.Request) {
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		snapshot, err := connector.ListRelationships(r.Context(), h.dossierRef())
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		snapshot.Backend = connector.Metadata().ID
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	h.relationMu.Lock()
	snapshot := connectors.RelationshipSnapshot{Nodes: cloneRelationshipNodes(h.nodes), Edges: cloneRelationshipEdges(h.edges), Backend: "memory"}
	h.relationMu.Unlock()
	h.attachmentMu.Lock()
	for i := range snapshot.Nodes {
		snapshot.Nodes[i].AttachmentCount = len(h.attachments[snapshot.Nodes[i].ID])
	}
	h.attachmentMu.Unlock()
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) createRelationshipNode(w http.ResponseWriter, r *http.Request) {
	var node connectors.RelationshipNode
	if err := decodeJSON(r, &node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid relationship node")
		return
	}
	node.ID = newID("NODE")
	node.CoverAttachmentID = ""
	node.AttachmentCount = 0
	if err := normalizeRelationshipNode(&node); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		created, err := connector.CreateRelationshipNode(r.Context(), h.dossierRef(), node)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	h.relationMu.Lock()
	h.nodes = append(h.nodes, node)
	h.relationMu.Unlock()
	writeJSON(w, http.StatusCreated, node)
}

func (h *Handler) updateRelationshipNode(w http.ResponseWriter, r *http.Request) {
	var node connectors.RelationshipNode
	if err := decodeJSON(r, &node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid relationship node")
		return
	}
	node.ID = r.PathValue("id")
	existing, err := h.relationshipNode(r.Context(), node.ID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	node.Primary = existing.Primary
	node.CoverAttachmentID = existing.CoverAttachmentID
	node.AttachmentCount = existing.AttachmentCount
	if err := normalizeRelationshipNode(&node); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		updated, err := connector.UpdateRelationshipNode(r.Context(), h.dossierRef(), node)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	for i := range h.nodes {
		if h.nodes[i].ID == node.ID {
			h.nodes[i] = node
			writeJSON(w, http.StatusOK, node)
			return
		}
	}
	writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
}

func (h *Handler) deleteRelationshipNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snapshot, err := h.relationshipSnapshot(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	for _, node := range snapshot.Nodes {
		if node.ID == id && node.Primary {
			writeError(w, http.StatusConflict, "primary object cannot be deleted")
			return
		}
	}
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		if err := connector.DeleteRelationshipNode(r.Context(), h.dossierRef(), id); err != nil {
			writeConnectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	removed := false
	for i := len(h.nodes) - 1; i >= 0; i-- {
		if h.nodes[i].ID == id {
			h.nodes = append(h.nodes[:i], h.nodes[i+1:]...)
			removed = true
		}
	}
	for i := len(h.edges) - 1; i >= 0; i-- {
		if h.edges[i].SourceID == id || h.edges[i].TargetID == id {
			h.edges = append(h.edges[:i], h.edges[i+1:]...)
		}
	}
	h.attachmentMu.Lock()
	delete(h.attachments, id)
	h.attachmentMu.Unlock()
	if !removed {
		writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRelationshipAttachments(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if err := h.requireRelationshipNode(r.Context(), nodeID); err != nil {
		writeConnectorError(w, err)
		return
	}
	if connector, ok := h.activeAttachmentConnector(r.Context()); ok {
		items, err := connector.ListRelationshipAttachments(r.Context(), h.dossierRef(), nodeID)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	h.attachmentMu.Lock()
	items := make([]connectors.RelationshipAttachment, 0, len(h.attachments[nodeID]))
	for _, item := range h.attachments[nodeID] {
		items = append(items, item.Metadata)
	}
	h.attachmentMu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt < items[j].CreatedAt })
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) createRelationshipAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, connectors.MaxRelationshipAttachmentSize+(1<<20))
	if err := r.ParseMultipartForm(connectors.MaxRelationshipAttachmentSize); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment exceeds 10 MiB")
		return
	}
	ref := h.dossierRef()
	if r.FormValue("caseId") != ref.CaseID {
		writeError(w, http.StatusConflict, "active case changed; reload before uploading")
		return
	}
	nodeID := r.PathValue("id")
	if err := h.requireRelationshipNode(r.Context(), nodeID); err != nil {
		writeConnectorError(w, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if err := validateRelationshipAttachmentFilename(header.Filename); err != nil {
		writeConnectorError(w, err)
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, connectors.MaxRelationshipAttachmentSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read attachment")
		return
	}
	if int64(len(content)) > connectors.MaxRelationshipAttachmentSize {
		writeConnectorError(w, connectors.ErrAttachmentLarge)
		return
	}
	mediaType := http.DetectContentType(content)
	if isRelationshipCoverImage(mediaType) && !validRelationshipImage(content, mediaType) {
		writeError(w, http.StatusBadRequest, "invalid image content")
		return
	}
	attachment := connectors.RelationshipAttachment{ID: newID("ATT"), NodeID: nodeID, Filename: header.Filename, MediaType: mediaType, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if connector, ok := h.activeAttachmentConnector(r.Context()); ok {
		created, storeErr := connector.StoreRelationshipAttachment(r.Context(), ref, attachment, content)
		if storeErr != nil {
			writeConnectorError(w, storeErr)
			return
		}
		if coverErr := h.assignFirstRelationshipCover(r.Context(), nodeID, created); coverErr != nil {
			if rollbackErr := connector.DeleteRelationshipAttachment(r.Context(), ref, nodeID, created.ID); rollbackErr != nil {
				writeError(w, http.StatusBadGateway, fmt.Sprintf("assign cover: %v; rollback attachment: %v", coverErr, rollbackErr))
				return
			}
			writeConnectorError(w, coverErr)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	h.attachmentMu.Lock()
	if len(h.attachments[nodeID]) >= connectors.MaxRelationshipAttachmentsPerNode {
		h.attachmentMu.Unlock()
		writeConnectorError(w, connectors.ErrAttachmentLimit)
		return
	}
	sum := sha256.Sum256(content)
	attachment.Size, attachment.SHA256 = int64(len(content)), hex.EncodeToString(sum[:])
	if h.attachments[nodeID] == nil {
		h.attachments[nodeID] = make(map[string]memoryAttachment)
	}
	h.attachments[nodeID][attachment.ID] = memoryAttachment{Metadata: attachment, Content: append([]byte{}, content...)}
	h.attachmentMu.Unlock()
	if coverErr := h.assignFirstRelationshipCover(r.Context(), nodeID, attachment); coverErr != nil {
		h.attachmentMu.Lock()
		delete(h.attachments[nodeID], attachment.ID)
		h.attachmentMu.Unlock()
		writeConnectorError(w, coverErr)
		return
	}
	writeJSON(w, http.StatusCreated, attachment)
}

func (h *Handler) downloadRelationshipAttachment(w http.ResponseWriter, r *http.Request) {
	nodeID, attachmentID := r.PathValue("id"), r.PathValue("attachmentId")
	var metadata connectors.RelationshipAttachment
	var content []byte
	var err error
	if connector, ok := h.activeAttachmentConnector(r.Context()); ok {
		metadata, content, err = connector.ReadRelationshipAttachment(r.Context(), h.dossierRef(), nodeID, attachmentID)
	} else {
		h.attachmentMu.Lock()
		item, exists := h.attachments[nodeID][attachmentID]
		h.attachmentMu.Unlock()
		if !exists {
			err = connectors.ErrEntityAbsent
		} else {
			metadata, content = item.Metadata, append([]byte{}, item.Content...)
		}
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	dispositionType := "attachment"
	if r.URL.Query().Get("inline") == "1" && isRelationshipCoverImage(metadata.MediaType) {
		dispositionType = "inline"
	}
	disposition := mime.FormatMediaType(dispositionType, map[string]string{"filename": metadata.Filename})
	if parsed, _, parseErr := mime.ParseMediaType(metadata.MediaType); parseErr == nil {
		metadata.MediaType = parsed
	} else {
		metadata.MediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", metadata.MediaType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", fmtInt64(metadata.Size))
	w.Header().Set("X-Content-SHA256", metadata.SHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handler) deleteRelationshipAttachment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID string `json:"caseId"`
	}
	if decodeJSON(r, &input) != nil || input.CaseID != h.dossierRef().CaseID {
		writeError(w, http.StatusConflict, "active case changed; reload before deleting")
		return
	}
	nodeID, attachmentID := r.PathValue("id"), r.PathValue("attachmentId")
	h.coverMu.Lock()
	defer h.coverMu.Unlock()
	node, err := h.relationshipNode(r.Context(), nodeID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if node.CoverAttachmentID == attachmentID {
		items, listErr := h.relationshipAttachments(r.Context(), nodeID)
		if listErr != nil {
			writeConnectorError(w, listErr)
			return
		}
		nextCover := ""
		for _, item := range items {
			if item.ID != attachmentID && isRelationshipCoverImage(item.MediaType) {
				nextCover = item.ID
				break
			}
		}
		if _, updateErr := h.updateRelationshipCover(r.Context(), node, nextCover); updateErr != nil {
			writeConnectorError(w, updateErr)
			return
		}
	}
	if connector, ok := h.activeAttachmentConnector(r.Context()); ok {
		if err := connector.DeleteRelationshipAttachment(r.Context(), h.dossierRef(), nodeID, attachmentID); err != nil {
			writeConnectorError(w, err)
			return
		}
	} else {
		h.attachmentMu.Lock()
		if _, exists := h.attachments[nodeID][attachmentID]; !exists {
			h.attachmentMu.Unlock()
			writeConnectorError(w, connectors.ErrEntityAbsent)
			return
		}
		delete(h.attachments[nodeID], attachmentID)
		h.attachmentMu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setRelationshipCover(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID       string `json:"caseId"`
		AttachmentID string `json:"attachmentId"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid cover request")
		return
	}
	if input.CaseID != h.dossierRef().CaseID {
		writeError(w, http.StatusConflict, "active case changed; reload before changing the cover")
		return
	}
	nodeID := r.PathValue("id")
	h.coverMu.Lock()
	defer h.coverMu.Unlock()
	node, err := h.relationshipNode(r.Context(), nodeID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if input.AttachmentID != "" {
		attachment, _, readErr := h.relationshipAttachment(r.Context(), nodeID, input.AttachmentID)
		if readErr != nil {
			writeConnectorError(w, readErr)
			return
		}
		if !isRelationshipCoverImage(attachment.MediaType) {
			writeError(w, http.StatusBadRequest, "cover attachment must be JPEG, PNG, WebP, or GIF")
			return
		}
	}
	updated, err := h.updateRelationshipCover(r.Context(), node, input.AttachmentID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) assignFirstRelationshipCover(ctx context.Context, nodeID string, attachment connectors.RelationshipAttachment) error {
	if !isRelationshipCoverImage(attachment.MediaType) {
		return nil
	}
	h.coverMu.Lock()
	defer h.coverMu.Unlock()
	node, err := h.relationshipNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.CoverAttachmentID != "" {
		current, _, readErr := h.relationshipAttachment(ctx, nodeID, node.CoverAttachmentID)
		if readErr == nil && isRelationshipCoverImage(current.MediaType) {
			return nil
		}
	}
	_, err = h.updateRelationshipCover(ctx, node, attachment.ID)
	return err
}

func (h *Handler) updateRelationshipCover(ctx context.Context, node connectors.RelationshipNode, attachmentID string) (connectors.RelationshipNode, error) {
	node.CoverAttachmentID = attachmentID
	if connector, ok := h.activeRelationshipConnector(ctx); ok {
		return connector.UpdateRelationshipNode(ctx, h.dossierRef(), node)
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	for i := range h.nodes {
		if h.nodes[i].ID == node.ID {
			h.nodes[i].CoverAttachmentID = attachmentID
			return h.nodes[i], nil
		}
	}
	return connectors.RelationshipNode{}, connectors.ErrEntityAbsent
}

func (h *Handler) relationshipNode(ctx context.Context, nodeID string) (connectors.RelationshipNode, error) {
	snapshot, err := h.relationshipSnapshot(ctx)
	if err != nil {
		return connectors.RelationshipNode{}, err
	}
	for _, node := range snapshot.Nodes {
		if node.ID == nodeID {
			return node, nil
		}
	}
	return connectors.RelationshipNode{}, connectors.ErrEntityAbsent
}

func (h *Handler) relationshipAttachments(ctx context.Context, nodeID string) ([]connectors.RelationshipAttachment, error) {
	if connector, ok := h.activeAttachmentConnector(ctx); ok {
		return connector.ListRelationshipAttachments(ctx, h.dossierRef(), nodeID)
	}
	h.attachmentMu.Lock()
	items := make([]connectors.RelationshipAttachment, 0, len(h.attachments[nodeID]))
	for _, item := range h.attachments[nodeID] {
		items = append(items, item.Metadata)
	}
	h.attachmentMu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt < items[j].CreatedAt })
	return items, nil
}

func (h *Handler) relationshipAttachment(ctx context.Context, nodeID, attachmentID string) (connectors.RelationshipAttachment, []byte, error) {
	if connector, ok := h.activeAttachmentConnector(ctx); ok {
		return connector.ReadRelationshipAttachment(ctx, h.dossierRef(), nodeID, attachmentID)
	}
	h.attachmentMu.Lock()
	item, exists := h.attachments[nodeID][attachmentID]
	h.attachmentMu.Unlock()
	if !exists {
		return connectors.RelationshipAttachment{}, nil, connectors.ErrEntityAbsent
	}
	return item.Metadata, append([]byte{}, item.Content...), nil
}

func isRelationshipCoverImage(mediaType string) bool {
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func validRelationshipImage(content []byte, mediaType string) bool {
	if mediaType != "image/webp" {
		_, format, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			return false
		}
		expected := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/gif": "gif"}
		return expected[mediaType] == format
	}
	if len(content) < 25 || string(content[:4]) != "RIFF" || string(content[8:12]) != "WEBP" || int64(binary.LittleEndian.Uint32(content[4:8]))+8 > int64(len(content)) {
		return false
	}
	switch string(content[12:16]) {
	case "VP8X":
		return len(content) >= 30
	case "VP8L":
		return len(content) >= 25 && content[20] == 0x2f
	case "VP8 ":
		return len(content) >= 30 && bytes.Equal(content[23:26], []byte{0x9d, 0x01, 0x2a})
	default:
		return false
	}
}

func (h *Handler) activeAttachmentConnector(ctx context.Context) (connectors.RelationshipAttachmentConnector, bool) {
	connector, ok := h.activeRelationshipConnector(ctx)
	if !ok {
		return nil, false
	}
	if !hasCapability(connector.Metadata().Capabilities, "relationships.attachments.read") {
		return nil, false
	}
	attachments, supported := connector.(connectors.RelationshipAttachmentConnector)
	return attachments, supported
}

func (h *Handler) requireRelationshipNode(ctx context.Context, nodeID string) error {
	_, err := h.relationshipNode(ctx, nodeID)
	return err
}

func validateRelationshipAttachmentFilename(name string) error {
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

func fmtInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (h *Handler) createRelationshipEdge(w http.ResponseWriter, r *http.Request) {
	var edge connectors.RelationshipEdge
	if err := decodeJSON(r, &edge); err != nil {
		writeError(w, http.StatusBadRequest, "invalid relationship edge")
		return
	}
	edge.ID = newID("REL")
	if err := h.normalizeRelationshipEdge(r.Context(), &edge); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		created, err := connector.CreateRelationshipEdge(r.Context(), h.dossierRef(), edge)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	h.relationMu.Lock()
	h.edges = append(h.edges, edge)
	h.relationMu.Unlock()
	writeJSON(w, http.StatusCreated, edge)
}

func (h *Handler) updateRelationshipEdge(w http.ResponseWriter, r *http.Request) {
	var edge connectors.RelationshipEdge
	if err := decodeJSON(r, &edge); err != nil {
		writeError(w, http.StatusBadRequest, "invalid relationship edge")
		return
	}
	edge.ID = r.PathValue("id")
	if err := h.normalizeRelationshipEdge(r.Context(), &edge); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		updated, err := connector.UpdateRelationshipEdge(r.Context(), h.dossierRef(), edge)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	for i := range h.edges {
		if h.edges[i].ID == edge.ID {
			h.edges[i] = edge
			writeJSON(w, http.StatusOK, edge)
			return
		}
	}
	writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
}

func (h *Handler) deleteRelationshipEdge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if connector, ok := h.activeRelationshipConnector(r.Context()); ok {
		if err := connector.DeleteRelationshipEdge(r.Context(), h.dossierRef(), id); err != nil {
			writeConnectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	for i := range h.edges {
		if h.edges[i].ID == id {
			h.edges = append(h.edges[:i], h.edges[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
}

func (h *Handler) activeRelationshipConnector(ctx context.Context) (connectors.RelationshipConnector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "relationships.read") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		if err == nil {
			graph, ok := connector.(connectors.RelationshipConnector)
			return graph, ok
		}
	}
	return nil, false
}

func (h *Handler) relationshipSnapshot(ctx context.Context) (connectors.RelationshipSnapshot, error) {
	if connector, ok := h.activeRelationshipConnector(ctx); ok {
		return connector.ListRelationships(ctx, h.dossierRef())
	}
	h.relationMu.Lock()
	defer h.relationMu.Unlock()
	return connectors.RelationshipSnapshot{Nodes: cloneRelationshipNodes(h.nodes), Edges: cloneRelationshipEdges(h.edges), Backend: "memory"}, nil
}

func normalizeRelationshipNode(node *connectors.RelationshipNode) error {
	node.Type, node.Title, node.Subtitle, node.Details, node.Risk = limited(node.Type, 32), limited(node.Title, 160), limited(node.Subtitle, 240), limited(node.Details, 4000), limited(node.Risk, 20)
	if !relationshipNodeTypes[node.Type] || node.Title == "" {
		return errors.New("valid node type and title are required")
	}
	if node.Risk == "" {
		node.Risk = "low"
	}
	if node.Risk != "low" && node.Risk != "medium" && node.Risk != "high" {
		return errors.New("invalid node risk")
	}
	if node.X < 0 || node.X > 1 || node.Y < 0 || node.Y > 1 {
		return errors.New("node position must be normalized")
	}
	node.SourceIDs = cleanStrings(node.SourceIDs, 30, 100)
	return nil
}

func (h *Handler) normalizeRelationshipEdge(ctx context.Context, edge *connectors.RelationshipEdge) error {
	edge.SourceID, edge.TargetID, edge.Label, edge.Note, edge.Kind = limited(edge.SourceID, 100), limited(edge.TargetID, 100), limited(edge.Label, 100), limited(edge.Note, 4000), limited(edge.Kind, 20)
	if edge.SourceID == "" || edge.TargetID == "" || edge.SourceID == edge.TargetID || edge.Label == "" {
		return errors.New("edge requires two different nodes and a label")
	}
	if edge.Kind == "" {
		edge.Kind = "standard"
	}
	if !relationshipKinds[edge.Kind] || edge.Confidence < 0 || edge.Confidence > 100 {
		return errors.New("invalid edge kind or confidence")
	}
	edge.SourceIDs = cleanStrings(edge.SourceIDs, 30, 100)
	snapshot, err := h.relationshipSnapshot(ctx)
	if err != nil {
		return err
	}
	foundSource, foundTarget := false, false
	for _, node := range snapshot.Nodes {
		foundSource = foundSource || node.ID == edge.SourceID
		foundTarget = foundTarget || node.ID == edge.TargetID
	}
	if !foundSource || !foundTarget {
		return errors.New("edge node does not exist")
	}
	return nil
}

func seedRelationships(value casefile.Case) ([]connectors.RelationshipNode, []connectors.RelationshipEdge) {
	primary := connectors.RelationshipNode{ID: newID("NODE"), Type: "object", Title: value.Subject.Codename, Subtitle: value.Subject.DisplayName, Details: value.Objective, Risk: value.Subject.Risk, SourceIDs: []string{}, X: .5, Y: .5, Primary: true}
	nodes := []connectors.RelationshipNode{primary}
	edges := []connectors.RelationshipEdge{}
	for i, identifier := range value.Subject.Identifiers {
		nodeType := "fact"
		lower := strings.ToLower(identifier.Type)
		if strings.Contains(lower, "account") || strings.Contains(lower, "user") || strings.Contains(lower, "email") || strings.Contains(lower, "phone") {
			nodeType = "account"
		}
		if strings.Contains(lower, "ip") || strings.Contains(lower, "domain") || strings.Contains(lower, "host") {
			nodeType = "infrastructure"
		}
		node := connectors.RelationshipNode{ID: newID("NODE"), Type: nodeType, Title: identifier.Value, Subtitle: strings.ToUpper(identifier.Type), Risk: "low", SourceIDs: []string{}, X: .18 + float64(i%3)*.16, Y: .78}
		nodes = append(nodes, node)
		edges = append(edges, connectors.RelationshipEdge{ID: newID("REL"), SourceID: primary.ID, TargetID: node.ID, Label: "IDENTIFIED BY", Confidence: value.Subject.Confidence, SourceIDs: []string{}, Kind: "evidence"})
	}
	for i, relation := range value.Subject.Relations {
		nodeType := relation.Type
		if !relationshipNodeTypes[nodeType] {
			nodeType = "subject"
		}
		node := connectors.RelationshipNode{ID: newID("NODE"), Type: nodeType, Title: relation.Name, Risk: relation.Risk, SourceIDs: []string{}, X: .2 + float64(i%4)*.2, Y: .18}
		nodes = append(nodes, node)
		kind := "standard"
		if relation.Risk == "high" {
			kind = "critical"
		}
		edges = append(edges, connectors.RelationshipEdge{ID: newID("REL"), SourceID: primary.ID, TargetID: node.ID, Label: "RELATED", Confidence: value.Subject.Confidence, SourceIDs: []string{}, Kind: kind})
	}
	return nodes, edges
}

func cloneRelationshipNodes(items []connectors.RelationshipNode) []connectors.RelationshipNode {
	result := append([]connectors.RelationshipNode{}, items...)
	for i := range result {
		result[i].SourceIDs = append([]string{}, result[i].SourceIDs...)
	}
	return result
}
func cloneRelationshipEdges(items []connectors.RelationshipEdge) []connectors.RelationshipEdge {
	result := append([]connectors.RelationshipEdge{}, items...)
	for i := range result {
		result[i].SourceIDs = append([]string{}, result[i].SourceIDs...)
	}
	return result
}
