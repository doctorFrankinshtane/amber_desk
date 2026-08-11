package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"amberdesk/pkg/connectors"
)

type relationshipNodeDocument struct {
	AmberDesk                   documentHeader `yaml:"amber_desk"`
	connectors.RelationshipNode `yaml:",inline"`
}

type relationshipEdgeDocument struct {
	AmberDesk                   documentHeader `yaml:"amber_desk"`
	connectors.RelationshipEdge `yaml:",inline"`
}

func (c *Connector) ListRelationships(ctx context.Context, ref connectors.DossierRef) (connectors.RelationshipSnapshot, error) {
	nodes, err := c.listRelationshipNodes(ref)
	if err != nil {
		return connectors.RelationshipSnapshot{}, err
	}
	edges, err := c.listRelationshipEdges(ref)
	if err != nil {
		return connectors.RelationshipSnapshot{}, err
	}
	for i := range nodes {
		attachments, listErr := c.ListRelationshipAttachments(ctx, ref, nodes[i].ID)
		if listErr != nil {
			return connectors.RelationshipSnapshot{}, listErr
		}
		nodes[i].AttachmentCount = len(attachments)
	}
	return connectors.RelationshipSnapshot{Nodes: nodes, Edges: edges, Backend: ID}, nil
}

func (c *Connector) CreateRelationshipNode(_ context.Context, ref connectors.DossierRef, node connectors.RelationshipNode) (connectors.RelationshipNode, error) {
	if node.ID == "" {
		id, err := documentID("NODE")
		if err != nil {
			return node, err
		}
		node.ID = id
	}
	path, err := c.relationshipPath(ref, "Nodes", node.ID, true)
	if err != nil {
		return node, err
	}
	if _, err := os.Stat(path); err == nil {
		return node, connectors.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return node, err
	}
	return node, c.writeRelationshipNode(ref, node)
}

func (c *Connector) UpdateRelationshipNode(_ context.Context, ref connectors.DossierRef, node connectors.RelationshipNode) (connectors.RelationshipNode, error) {
	path, err := c.relationshipPath(ref, "Nodes", node.ID, false)
	if err != nil {
		return node, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return node, connectors.ErrEntityAbsent
	} else if err != nil {
		return node, err
	}
	return node, c.writeRelationshipNode(ref, node)
}

func (c *Connector) DeleteRelationshipNode(ctx context.Context, ref connectors.DossierRef, id string) error {
	node, err := c.readRelationshipNode(ref, id)
	if err != nil {
		return err
	}
	if node.Primary {
		return errors.New("primary object cannot be deleted")
	}
	if err := c.trashRelationshipAttachments(ref, id); err != nil {
		return err
	}
	path, err := c.relationshipPath(ref, "Nodes", id, false)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return connectors.ErrEntityAbsent
	} else if err != nil {
		return err
	}
	snapshot, err := c.ListRelationships(ctx, ref)
	if err != nil {
		return err
	}
	for _, edge := range snapshot.Edges {
		if edge.SourceID == id || edge.TargetID == id {
			if err := c.DeleteRelationshipEdge(ctx, ref, edge.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Connector) CreateRelationshipEdge(_ context.Context, ref connectors.DossierRef, edge connectors.RelationshipEdge) (connectors.RelationshipEdge, error) {
	if edge.ID == "" {
		id, err := documentID("REL")
		if err != nil {
			return edge, err
		}
		edge.ID = id
	}
	path, err := c.relationshipPath(ref, "Edges", edge.ID, true)
	if err != nil {
		return edge, err
	}
	if _, err := os.Stat(path); err == nil {
		return edge, connectors.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return edge, err
	}
	return edge, c.writeRelationshipEdge(ref, edge)
}

func (c *Connector) UpdateRelationshipEdge(_ context.Context, ref connectors.DossierRef, edge connectors.RelationshipEdge) (connectors.RelationshipEdge, error) {
	path, err := c.relationshipPath(ref, "Edges", edge.ID, false)
	if err != nil {
		return edge, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return edge, connectors.ErrEntityAbsent
	} else if err != nil {
		return edge, err
	}
	return edge, c.writeRelationshipEdge(ref, edge)
}

func (c *Connector) DeleteRelationshipEdge(_ context.Context, ref connectors.DossierRef, id string) error {
	path, err := c.relationshipPath(ref, "Edges", id, false)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return connectors.ErrEntityAbsent
	} else {
		return err
	}
}

func (c *Connector) listRelationshipNodes(ref connectors.DossierRef) ([]connectors.RelationshipNode, error) {
	directory, err := c.caseDirectory(ref, false, "Relations", "Nodes")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []connectors.RelationshipNode{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]connectors.RelationshipNode, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var document relationshipNodeDocument
		if err := unmarshalNote(data, &document); err != nil {
			return nil, fmt.Errorf("parse relationship node %s: %w", entry.Name(), err)
		}
		if document.AmberDesk.valid(documentKindRelationshipNode, ref.CaseID) {
			document.SourceIDs = append([]string{}, document.SourceIDs...)
			result = append(result, document.RelationshipNode)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Primary != result[j].Primary {
			return result[i].Primary
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (c *Connector) listRelationshipEdges(ref connectors.DossierRef) ([]connectors.RelationshipEdge, error) {
	directory, err := c.caseDirectory(ref, false, "Relations", "Edges")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []connectors.RelationshipEdge{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]connectors.RelationshipEdge, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var document relationshipEdgeDocument
		if err := unmarshalNote(data, &document); err != nil {
			return nil, fmt.Errorf("parse relationship edge %s: %w", entry.Name(), err)
		}
		if document.AmberDesk.valid(documentKindRelationshipEdge, ref.CaseID) {
			document.SourceIDs = append([]string{}, document.SourceIDs...)
			result = append(result, document.RelationshipEdge)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *Connector) readRelationshipNode(ref connectors.DossierRef, id string) (connectors.RelationshipNode, error) {
	path, err := c.relationshipPath(ref, "Nodes", id, false)
	if err != nil {
		return connectors.RelationshipNode{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return connectors.RelationshipNode{}, connectors.ErrEntityAbsent
	}
	if err != nil {
		return connectors.RelationshipNode{}, err
	}
	var document relationshipNodeDocument
	if err := unmarshalNote(data, &document); err != nil {
		return connectors.RelationshipNode{}, err
	}
	if !document.AmberDesk.valid(documentKindRelationshipNode, ref.CaseID) {
		return connectors.RelationshipNode{}, connectors.ErrEntityAbsent
	}
	return document.RelationshipNode, nil
}

func (c *Connector) writeRelationshipNode(ref connectors.DossierRef, node connectors.RelationshipNode) error {
	path, err := c.relationshipPath(ref, "Nodes", node.ID, true)
	if err != nil {
		return err
	}
	document := relationshipNodeDocument{AmberDesk: newDocumentHeader(documentKindRelationshipNode, ref.CaseID), RelationshipNode: node}
	data, err := marshalNote(document, "# "+node.Title+"\n\n"+node.Details+"\n\n## Amber Desk\n\nRelationship node metadata is stored in YAML frontmatter.")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (c *Connector) writeRelationshipEdge(ref connectors.DossierRef, edge connectors.RelationshipEdge) error {
	path, err := c.relationshipPath(ref, "Edges", edge.ID, true)
	if err != nil {
		return err
	}
	document := relationshipEdgeDocument{AmberDesk: newDocumentHeader(documentKindRelationshipEdge, ref.CaseID), RelationshipEdge: edge}
	data, err := marshalNote(document, "# "+edge.Label+"\n\n"+edge.Note+"\n\n## Amber Desk\n\nRelationship edge metadata is stored in YAML frontmatter.")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (c *Connector) relationshipPath(ref connectors.DossierRef, kind, id string, create bool) (string, error) {
	directory, err := c.caseDirectory(ref, create, "Relations", kind)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, sanitize(id)+".md")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}
