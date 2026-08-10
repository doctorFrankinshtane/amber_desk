package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"amberdesk/internal/sherlock"
	"amberdesk/pkg/connectors"
)

func (h *Handler) sherlockStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.sherlock.Status(r.Context()))
}

func (h *Handler) startSherlockScan(w http.ResponseWriter, r *http.Request) {
	if !h.allowToolRequest(w, r) {
		return
	}
	var input struct {
		CaseID                   string `json:"caseId"`
		SourceNodeID             string `json:"sourceNodeId"`
		Username                 string `json:"username"`
		ExternalTrafficConfirmed bool   `json:"externalTrafficConfirmed"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid scan request")
		return
	}
	if !input.ExternalTrafficConfirmed {
		writeError(w, http.StatusBadRequest, "external traffic confirmation is required")
		return
	}
	ref := h.dossierRef()
	if input.CaseID != ref.CaseID {
		writeError(w, http.StatusConflict, "active case changed; reopen the Sherlock console")
		return
	}
	if _, err := h.relationshipNode(r.Context(), input.SourceNodeID); err != nil {
		writeConnectorError(w, err)
		return
	}
	status := h.sherlock.Status(r.Context())
	if !status.Ready {
		writeError(w, http.StatusServiceUnavailable, status.Message)
		return
	}
	scan, err := h.sherlock.Start(input.CaseID, input.SourceNodeID, input.Username)
	if err != nil {
		code := http.StatusConflict
		if strings.Contains(err.Error(), "global") {
			code = http.StatusTooManyRequests
		} else if strings.Contains(err.Error(), "username") {
			code = http.StatusBadRequest
		}
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, scan)
}

func (h *Handler) getSherlockScan(w http.ResponseWriter, r *http.Request) {
	scan, ok := h.sherlock.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	if scan.CaseID != h.dossierRef().CaseID {
		writeError(w, http.StatusConflict, "scan belongs to another dossier")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, scan)
}

func (h *Handler) cancelSherlockScan(w http.ResponseWriter, r *http.Request) {
	if !h.allowToolRequest(w, r) {
		return
	}
	scan, ok := h.sherlock.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	if scan.CaseID != h.dossierRef().CaseID {
		writeError(w, http.StatusConflict, "scan belongs to another dossier")
		return
	}
	updated, err := h.sherlock.Cancel(scan.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, updated)
}

func (h *Handler) streamSherlockScan(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "streaming is unavailable")
		return
	}
	snapshot, events, unsubscribe, ok := h.sherlock.Subscribe(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	defer unsubscribe()
	if snapshot.CaseID != h.dossierRef().CaseID {
		writeError(w, http.StatusConflict, "scan belongs to another dossier")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	writeSSE(w, "snapshot", snapshot)
	flusher.Flush()
	if snapshot.State == "completed" || snapshot.State == "failed" || snapshot.State == "cancelled" {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-events:
			writeSSE(w, event.Type, event)
			flusher.Flush()
			if event.Type == "completed" || event.Type == "failed" || event.Type == "cancelled" {
				if final, found := h.sherlock.Get(snapshot.ID); found {
					writeSSE(w, "snapshot", final)
					flusher.Flush()
				}
				return
			}
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, event string, value any) {
	data, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func (h *Handler) importSherlockScan(w http.ResponseWriter, r *http.Request) {
	if !h.allowToolRequest(w, r) {
		return
	}
	var input struct {
		CaseID    string   `json:"caseId"`
		ResultIDs []string `json:"resultIds"`
	}
	if decodeJSON(r, &input) != nil || len(input.ResultIDs) == 0 || len(input.ResultIDs) > 200 {
		writeError(w, http.StatusBadRequest, "select between 1 and 200 claimed results")
		return
	}
	scan, ok := h.sherlock.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	ref := h.dossierRef()
	if input.CaseID != ref.CaseID || scan.CaseID != ref.CaseID {
		writeError(w, http.StatusConflict, "active case changed; return to the scan dossier before importing")
		return
	}
	if scan.State != "completed" {
		writeError(w, http.StatusConflict, "only a completed scan can be imported")
		return
	}
	source, err := h.relationshipNode(r.Context(), scan.SourceNode)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	selectedSet := make(map[string]bool, len(input.ResultIDs))
	for _, id := range input.ResultIDs {
		if selectedSet[id] {
			writeError(w, http.StatusBadRequest, "duplicate result selection")
			return
		}
		selectedSet[id] = true
	}
	selected := make([]sherlock.Result, 0, len(input.ResultIDs))
	for _, result := range scan.Results {
		if selectedSet[result.ID] {
			if result.Status != "claimed" {
				writeError(w, http.StatusBadRequest, "only claimed profiles can be imported")
				return
			}
			selected = append(selected, result)
			delete(selectedSet, result.ID)
		}
	}
	if len(selectedSet) != 0 {
		writeError(w, http.StatusBadRequest, "one or more scan results are invalid")
		return
	}
	attachments, err := h.relationshipAttachments(r.Context(), source.ID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if len(attachments) >= connectors.MaxRelationshipAttachmentsPerNode {
		writeConnectorError(w, connectors.ErrAttachmentLimit)
		return
	}

	event, nodes, edges, attachment, err := h.commitSherlockImport(r.Context(), scan, source, selected)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"event": event, "nodes": nodes, "edges": edges, "attachment": attachment, "selected": len(selected)})
}

func (h *Handler) commitSherlockImport(ctx context.Context, scan sherlock.Scan, source connectors.RelationshipNode, selected []sherlock.Result) (connectors.TimelineEvent, []connectors.RelationshipNode, []connectors.RelationshipEdge, connectors.RelationshipAttachment, error) {
	now := time.Now().UTC()
	urls := make([]string, len(selected))
	for index, result := range selected {
		urls[index] = result.ProfileURL
	}
	event := connectors.TimelineEvent{ID: newID("EV"), OccurredAt: now.Format(time.RFC3339), Time: now.Format("15:04"), Date: strings.ToUpper(now.Format("02 Jan")), Type: "social", Title: "Sherlock scan: " + scan.Username, Summary: fmt.Sprintf("Selected %d of %d claimed profiles", len(selected), scan.Claimed), Source: "Sherlock " + sherlock.SupportedVersion, Confidence: 40, Status: "pending", Fingerprint: "SHERLOCK:" + scan.ID, Indicators: urls, Notes: []connectors.TimelineNote{}}
	reportContent, err := json.MarshalIndent(map[string]any{"provider": "Sherlock", "version": sherlock.SupportedVersion, "scanId": scan.ID, "caseId": scan.CaseID, "sourceNodeId": scan.SourceNode, "username": scan.Username, "createdAt": scan.CreatedAt, "completedAt": scan.CompletedAt, "results": scan.Results, "selectedResultIds": resultIDs(selected)}, "", "  ")
	if err != nil {
		return event, nil, nil, connectors.RelationshipAttachment{}, err
	}
	filename := fmt.Sprintf("sherlock-%s-%s.json", scan.Username, now.Format("20060102T150405Z"))
	attachment := connectors.RelationshipAttachment{ID: newID("ATT"), NodeID: source.ID, Filename: filename, MediaType: "application/json", CreatedAt: now.Format(time.RFC3339Nano)}
	createdAttachment, err := h.storeSherlockAttachment(ctx, attachment, reportContent)
	if err != nil {
		return event, nil, nil, attachment, fmt.Errorf("archive Sherlock report: %w", err)
	}
	createdEvent, err := h.storeSherlockTimelineEvent(ctx, event)
	if err != nil {
		h.deleteSherlockAttachment(ctx, createdAttachment)
		return event, nil, nil, createdAttachment, fmt.Errorf("create Sherlock timeline event: %w", err)
	}

	snapshot, err := h.relationshipSnapshot(ctx)
	if err != nil {
		h.rollbackSherlockImport(ctx, createdEvent, nil, nil, createdAttachment)
		return event, nil, nil, createdAttachment, err
	}
	existingByURL := make(map[string]connectors.RelationshipNode)
	for _, node := range snapshot.Nodes {
		if normalized := normalizeProfileURL(node.Subtitle); normalized != "" {
			existingByURL[normalized] = node
		}
	}
	createdNodes := []connectors.RelationshipNode{}
	createdEdges := []connectors.RelationshipEdge{}
	for index, result := range selected {
		node, exists := existingByURL[normalizeProfileURL(result.ProfileURL)]
		if !exists {
			angle := float64(index%8) / 8
			node = connectors.RelationshipNode{ID: newID("NODE"), Type: "account", Title: "@" + scan.Username + " / " + result.Site, Subtitle: result.ProfileURL, Details: "Sherlock candidate; requires manual verification.", Risk: "low", SourceIDs: []string{createdEvent.ID}, X: clampPosition(source.X + .22*float64Sign(index)*(.7+angle)), Y: clampPosition(source.Y + .16*float64Sign(index+2)*(.7+angle))}
			if err := normalizeRelationshipNode(&node); err != nil {
				h.rollbackSherlockImport(ctx, createdEvent, createdNodes, createdEdges, createdAttachment)
				return event, nil, nil, attachment, err
			}
			stored, storeErr := h.storeSherlockNode(ctx, node)
			if storeErr != nil {
				h.rollbackSherlockImport(ctx, createdEvent, createdNodes, createdEdges, createdAttachment)
				return event, nil, nil, attachment, storeErr
			}
			node = stored
			createdNodes = append(createdNodes, node)
			existingByURL[normalizeProfileURL(result.ProfileURL)] = node
		}
		if relationshipEdgeExists(snapshot.Edges, source.ID, node.ID, "FOUND ON") || relationshipEdgeExists(createdEdges, source.ID, node.ID, "FOUND ON") {
			continue
		}
		edge := connectors.RelationshipEdge{ID: newID("REL"), SourceID: source.ID, TargetID: node.ID, Label: "FOUND ON", Confidence: 40, SourceIDs: []string{createdEvent.ID}, Note: "Candidate profile discovered by Sherlock; verify identity independently.", Kind: "standard"}
		stored, storeErr := h.storeSherlockEdge(ctx, edge)
		if storeErr != nil {
			h.rollbackSherlockImport(ctx, createdEvent, createdNodes, createdEdges, createdAttachment)
			return event, nil, nil, attachment, storeErr
		}
		createdEdges = append(createdEdges, stored)
	}
	return createdEvent, createdNodes, createdEdges, createdAttachment, nil
}

func (h *Handler) storeSherlockTimelineEvent(ctx context.Context, event connectors.TimelineEvent) (connectors.TimelineEvent, error) {
	if connector, ok := h.activeTimelineConnector(ctx); ok {
		return connector.CreateTimelineEvent(ctx, h.dossierRef(), event)
	}
	created, err := h.store.AddEvent(timelineToCaseEvent(event))
	return caseEventToTimeline(created, 0), err
}

func (h *Handler) storeSherlockNode(ctx context.Context, node connectors.RelationshipNode) (connectors.RelationshipNode, error) {
	if connector, ok := h.activeRelationshipConnector(ctx); ok {
		return connector.CreateRelationshipNode(ctx, h.dossierRef(), node)
	}
	h.relationMu.Lock()
	h.nodes = append(h.nodes, node)
	h.relationMu.Unlock()
	return node, nil
}

func (h *Handler) storeSherlockEdge(ctx context.Context, edge connectors.RelationshipEdge) (connectors.RelationshipEdge, error) {
	if connector, ok := h.activeRelationshipConnector(ctx); ok {
		return connector.CreateRelationshipEdge(ctx, h.dossierRef(), edge)
	}
	h.relationMu.Lock()
	h.edges = append(h.edges, edge)
	h.relationMu.Unlock()
	return edge, nil
}

func (h *Handler) storeSherlockAttachment(ctx context.Context, attachment connectors.RelationshipAttachment, content []byte) (connectors.RelationshipAttachment, error) {
	if connector, ok := h.activeAttachmentConnector(ctx); ok {
		return connector.StoreRelationshipAttachment(ctx, h.dossierRef(), attachment, content)
	}
	sum := sha256.Sum256(content)
	attachment.Size, attachment.SHA256 = int64(len(content)), hex.EncodeToString(sum[:])
	h.attachmentMu.Lock()
	defer h.attachmentMu.Unlock()
	if len(h.attachments[attachment.NodeID]) >= connectors.MaxRelationshipAttachmentsPerNode {
		return attachment, connectors.ErrAttachmentLimit
	}
	if h.attachments[attachment.NodeID] == nil {
		h.attachments[attachment.NodeID] = make(map[string]memoryAttachment)
	}
	h.attachments[attachment.NodeID][attachment.ID] = memoryAttachment{Metadata: attachment, Content: append([]byte(nil), content...)}
	return attachment, nil
}

func (h *Handler) rollbackSherlockImport(ctx context.Context, event connectors.TimelineEvent, nodes []connectors.RelationshipNode, edges []connectors.RelationshipEdge, attachment connectors.RelationshipAttachment) {
	if connector, ok := h.activeRelationshipConnector(ctx); ok {
		for index := len(edges) - 1; index >= 0; index-- {
			_ = connector.DeleteRelationshipEdge(ctx, h.dossierRef(), edges[index].ID)
		}
		for index := len(nodes) - 1; index >= 0; index-- {
			_ = connector.DeleteRelationshipNode(ctx, h.dossierRef(), nodes[index].ID)
		}
	} else {
		h.relationMu.Lock()
		for _, edge := range edges {
			h.edges = removeEdge(h.edges, edge.ID)
		}
		for _, node := range nodes {
			h.nodes = removeNode(h.nodes, node.ID)
		}
		h.relationMu.Unlock()
	}
	if connector, ok := h.activeTimelineConnector(ctx); ok {
		if deleter, supported := connector.(connectors.TimelineDeleteConnector); supported {
			_ = deleter.DeleteTimelineEvent(ctx, h.dossierRef(), event.ID)
		}
	} else {
		_ = h.store.DeleteEvent(event.ID)
	}
	h.deleteSherlockAttachment(ctx, attachment)
}

func (h *Handler) deleteSherlockAttachment(ctx context.Context, attachment connectors.RelationshipAttachment) {
	if connector, ok := h.activeAttachmentConnector(ctx); ok {
		_ = connector.DeleteRelationshipAttachment(ctx, h.dossierRef(), attachment.NodeID, attachment.ID)
		return
	}
	h.attachmentMu.Lock()
	delete(h.attachments[attachment.NodeID], attachment.ID)
	h.attachmentMu.Unlock()
}

func (h *Handler) allowToolRequest(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "application/json is required")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
			writeError(w, http.StatusForbidden, "foreign origin is not allowed")
			return false
		}
	}
	if !h.allowRemoteTools {
		host := r.Host
		if parsedHost, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsedHost
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && net.ParseIP(host) == nil {
			writeError(w, http.StatusForbidden, "tool execution is limited to localhost")
			return false
		}
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
			writeError(w, http.StatusForbidden, "tool execution is limited to localhost")
			return false
		}
	}
	return true
}

func normalizeProfileURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	return parsed.String()
}

func relationshipEdgeExists(edges []connectors.RelationshipEdge, source, target, label string) bool {
	for _, edge := range edges {
		if edge.SourceID == source && edge.TargetID == target && strings.EqualFold(edge.Label, label) {
			return true
		}
	}
	return false
}

func resultIDs(results []sherlock.Result) []string {
	ids := make([]string, len(results))
	for i := range results {
		ids[i] = results[i].ID
	}
	return ids
}
func clampPosition(value float64) float64 {
	if value < .08 {
		return .08
	}
	if value > .92 {
		return .92
	}
	return value
}
func float64Sign(index int) float64 {
	if index%2 == 0 {
		return 1
	}
	return -1
}
func removeNode(items []connectors.RelationshipNode, id string) []connectors.RelationshipNode {
	for i := range items {
		if items[i].ID == id {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}
func removeEdge(items []connectors.RelationshipEdge, id string) []connectors.RelationshipEdge {
	for i := range items {
		if items[i].ID == id {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}
