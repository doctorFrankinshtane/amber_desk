package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
			if h.nodes[i].Primary {
				node.Primary = true
			}
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
	mediaType := header.Header.Get("Content-Type")
	parsedMediaType, _, mediaErr := mime.ParseMediaType(mediaType)
	if mediaErr != nil || parsedMediaType == "application/octet-stream" {
		mediaType = http.DetectContentType(content)
	} else {
		mediaType = parsedMediaType
	}
	attachment := connectors.RelationshipAttachment{ID: newID("ATT"), NodeID: nodeID, Filename: header.Filename, MediaType: mediaType, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if connector, ok := h.activeAttachmentConnector(r.Context()); ok {
		created, storeErr := connector.StoreRelationshipAttachment(r.Context(), ref, attachment, content)
		if storeErr != nil {
			writeConnectorError(w, storeErr)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	h.attachmentMu.Lock()
	defer h.attachmentMu.Unlock()
	if len(h.attachments[nodeID]) >= connectors.MaxRelationshipAttachmentsPerNode {
		writeConnectorError(w, connectors.ErrAttachmentLimit)
		return
	}
	sum := sha256.Sum256(content)
	attachment.Size, attachment.SHA256 = int64(len(content)), hex.EncodeToString(sum[:])
	if h.attachments[nodeID] == nil {
		h.attachments[nodeID] = make(map[string]memoryAttachment)
	}
	h.attachments[nodeID][attachment.ID] = memoryAttachment{Metadata: attachment, Content: append([]byte{}, content...)}
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
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": metadata.Filename})
	if parsed, _, parseErr := mime.ParseMediaType(metadata.MediaType); parseErr == nil {
		metadata.MediaType = parsed
	} else {
		metadata.MediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", metadata.MediaType)
	w.Header().Set("Content-Disposition", disposition)
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
	snapshot, err := h.relationshipSnapshot(ctx)
	if err != nil {
		return err
	}
	for _, node := range snapshot.Nodes {
		if node.ID == nodeID {
			return nil
		}
	}
	return connectors.ErrEntityAbsent
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
