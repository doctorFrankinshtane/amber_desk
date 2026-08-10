package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"amberdesk/internal/casefile"
	"amberdesk/internal/connectors"
	"amberdesk/internal/connectors/obsidian"
	"amberdesk/internal/httpapi"
)

func TestCaseAndEventWorkflow(t *testing.T) {
	handler := newHandler()

	caseResponse := request(t, handler, http.MethodGet, "/api/case", "")
	if caseResponse.Code != http.StatusOK {
		t.Fatalf("GET /api/case status = %d", caseResponse.Code)
	}
	var caseData casefile.Case
	decode(t, caseResponse, &caseData)
	if caseData.Name != "UNTITLED CASE" || len(caseData.Events) != 0 {
		t.Fatalf("unexpected case response: %+v", caseData)
	}
	if caseData.Subject.Aliases == nil || caseData.Subject.Identifiers == nil || caseData.Subject.Relations == nil || caseData.Tags == nil {
		t.Fatalf("empty collections must serialize as arrays: %+v", caseData)
	}
	createdResponse := request(t, handler, http.MethodPost, "/api/timeline/events", `{"title":"Test observation","type":"identity","confidence":70}`)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create event = %d: %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created casefile.Event
	decode(t, createdResponse, &created)

	statusResponse := request(t, handler, http.MethodPatch, "/api/events/"+created.ID+"/status", `{"status":"verified"}`)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var updated casefile.Event
	decode(t, statusResponse, &updated)
	if updated.Status != "verified" {
		t.Fatalf("event status = %q", updated.Status)
	}

	noteResponse := request(t, handler, http.MethodPost, "/api/events/"+created.ID+"/notes", `{"text":"Independent confirmation"}`)
	if noteResponse.Code != http.StatusCreated {
		t.Fatalf("POST note status = %d: %s", noteResponse.Code, noteResponse.Body.String())
	}
	decode(t, noteResponse, &updated)
	if len(updated.Notes) != 1 || updated.Notes[0].Text != "Independent confirmation" {
		t.Fatalf("unexpected notes: %+v", updated.Notes)
	}
}

func TestStatusValidation(t *testing.T) {
	handler := newHandler()
	response := request(t, handler, http.MethodPatch, "/api/events/EV-108/status", `{"status":"discarded"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateDossierSeedsRelationshipBoard(t *testing.T) {
	handler := newHandler()
	body := `{"name":"CASE ORION","owner":"Analyst","objective":"Trace infrastructure","tags":["person"],"subject":{"codename":"ORION","displayName":"Test Subject","risk":"high","confidence":75,"aliases":[],"identifiers":[{"type":"email","value":"orion@example.test"}],"relations":[{"name":"VECTOR LLC","type":"organization","risk":"high"}]}}`
	created := request(t, handler, http.MethodPost, "/api/case", body)
	if created.Code != http.StatusCreated {
		t.Fatalf("create dossier: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		Case          casefile.Case                   `json:"case"`
		Relationships connectors.RelationshipSnapshot `json:"relationships"`
	}
	decode(t, created, &response)
	if response.Case.Name != "CASE ORION" || !strings.HasPrefix(response.Case.ID, "CASE-") || len(response.Case.ID) != 13 {
		t.Fatalf("unexpected case: %+v", response.Case)
	}
	if len(response.Relationships.Nodes) != 3 || len(response.Relationships.Edges) != 2 || !response.Relationships.Nodes[0].Primary {
		t.Fatalf("unexpected relationship seed: %+v", response.Relationships)
	}
	edge := request(t, handler, http.MethodPost, "/api/relationships/edges", `{"sourceId":"missing","targetId":"also-missing","label":"CONTROLS","confidence":50}`)
	if edge.Code != http.StatusBadRequest {
		t.Fatalf("invalid edge status = %d", edge.Code)
	}
	duplicate := request(t, handler, http.MethodPost, "/api/case", body)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("second active dossier status = %d", duplicate.Code)
	}
}

func TestCreateDossierSynchronizesObsidian(t *testing.T) {
	vault := t.TempDir()
	provider, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)
	body := `{"name":"CASE ORION","subject":{"codename":"ORION","risk":"medium","confidence":75,"aliases":[],"identifiers":[],"relations":[{"name":"VECTOR LLC","type":"organization","risk":"high"}]},"tags":[]}`
	created := request(t, handler, http.MethodPost, "/api/case", body)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"state":"synced"`) {
		t.Fatalf("create synced dossier: %d %s", created.Code, created.Body.String())
	}
	dossiers, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Dossiers", "*.md"))
	nodes, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Cases", "CASE-*-CASE-ORION", "Relations", "Nodes", "*.md"))
	edges, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Cases", "CASE-*-CASE-ORION", "Relations", "Edges", "*.md"))
	if len(dossiers) != 1 || len(nodes) != 2 || len(edges) != 1 {
		t.Fatalf("synced files: dossiers=%v nodes=%v edges=%v", dossiers, nodes, edges)
	}
	stateConnector := any(provider).(connectors.WorkspaceStateConnector)
	state, err := stateConnector.ReadWorkspaceState(context.Background(), "active-case")
	if err != nil || !strings.Contains(string(state), `"name": "CASE ORION"`) {
		t.Fatalf("persisted active case: %v %s", err, state)
	}
}

func TestMemoryTimelineAndMapWorkflow(t *testing.T) {
	handler := newHandler()

	timeline := request(t, handler, http.MethodGet, "/api/timeline", "")
	if timeline.Code != http.StatusOK || !strings.Contains(timeline.Body.String(), `"backend":"memory"`) {
		t.Fatalf("timeline response: %d %s", timeline.Code, timeline.Body.String())
	}
	createdEvent := request(t, handler, http.MethodPost, "/api/timeline/events", `{"title":"Manual observation","type":"identity","confidence":70}`)
	if createdEvent.Code != http.StatusCreated {
		t.Fatalf("create timeline event: %d %s", createdEvent.Code, createdEvent.Body.String())
	}

	first := request(t, handler, http.MethodPost, "/api/map/markers", `{"label":"A","latitude":10,"longitude":20}`)
	second := request(t, handler, http.MethodPost, "/api/map/markers", `{"label":"B","latitude":30,"longitude":40}`)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("create markers: %d %d", first.Code, second.Code)
	}
	var markerA, markerB connectors.MapMarker
	decode(t, first, &markerA)
	decode(t, second, &markerB)
	routeBody := `{"fromMarkerId":"` + markerA.ID + `","toMarkerId":"` + markerB.ID + `","label":"A to B"}`
	route := request(t, handler, http.MethodPost, "/api/map/routes", routeBody)
	if route.Code != http.StatusCreated {
		t.Fatalf("create route: %d %s", route.Code, route.Body.String())
	}
	mapResponse := request(t, handler, http.MethodGet, "/api/map", "")
	if mapResponse.Code != http.StatusOK || !strings.Contains(mapResponse.Body.String(), `"backend":"memory"`) {
		t.Fatalf("map response: %d %s", mapResponse.Code, mapResponse.Body.String())
	}
}

func TestRoutesTimelineWritesThroughObsidianCapability(t *testing.T) {
	provider, err := obsidian.New(obsidian.Config{VaultPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)

	createdResponse := request(t, handler, http.MethodPost, "/api/timeline/events", `{"title":"Vault event","type":"identity","confidence":85}`)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create Obsidian event: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created connectors.TimelineEvent
	decode(t, createdResponse, &created)
	statusResponse := request(t, handler, http.MethodPatch, "/api/events/"+created.ID+"/status", `{"status":"verified"}`)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("update Obsidian event: %d %s", statusResponse.Code, statusResponse.Body.String())
	}
	timelineResponse := request(t, handler, http.MethodGet, "/api/timeline", "")
	if timelineResponse.Code != http.StatusOK || !strings.Contains(timelineResponse.Body.String(), `"backend":"obsidian"`) || !strings.Contains(timelineResponse.Body.String(), `"status":"verified"`) {
		t.Fatalf("Obsidian timeline: %d %s", timelineResponse.Code, timelineResponse.Body.String())
	}
}

func TestServesConfiguredLocalMapTiles(t *testing.T) {
	tileRoot := t.TempDir()
	tileDirectory := filepath.Join(tileRoot, "0", "0")
	if err := os.MkdirAll(tileDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	tile := []byte("local tile bytes")
	if err := os.WriteFile(filepath.Join(tileDirectory, "0.png"), tile, 0o600); err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	handler := httpapi.NewWithConfig(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(&fakeConnector{}), web, httpapi.Config{MapTiles: httpapi.MapTileConfig{Directory: tileRoot, Extension: "png", MaxZoom: 18}})

	configResponse := request(t, handler, http.MethodGet, "/api/map/basemap", "")
	if configResponse.Code != http.StatusOK || !strings.Contains(configResponse.Body.String(), `"mode":"local_xyz"`) {
		t.Fatalf("basemap config: %d %s", configResponse.Code, configResponse.Body.String())
	}
	tileResponse := request(t, handler, http.MethodGet, "/api/map/tiles/0/0/0", "")
	if tileResponse.Code != http.StatusOK || tileResponse.Body.String() != string(tile) {
		t.Fatalf("map tile: %d %q", tileResponse.Code, tileResponse.Body.String())
	}
	invalidResponse := request(t, handler, http.MethodGet, "/api/map/tiles/0/1/0", "")
	if invalidResponse.Code != http.StatusNotFound {
		t.Fatalf("invalid map tile = %d", invalidResponse.Code)
	}
}

func TestStaticIndex(t *testing.T) {
	handler := newHandler()
	response := request(t, handler, http.MethodGet, "/", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Amber Desk") {
		t.Fatalf("unexpected static response: %d %q", response.Code, response.Body.String())
	}
}

func TestIntegrationDossierWorkflow(t *testing.T) {
	handler := newHandler()

	listResponse := request(t, handler, http.MethodGet, "/api/integrations", "")
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"id":"test"`) {
		t.Fatalf("unexpected integrations response: %d %s", listResponse.Code, listResponse.Body.String())
	}

	readResponse := request(t, handler, http.MethodGet, "/api/integrations/test/dossier", "")
	if readResponse.Code != http.StatusOK || !strings.Contains(readResponse.Body.String(), "Initial dossier") {
		t.Fatalf("unexpected dossier response: %d %s", readResponse.Code, readResponse.Body.String())
	}

	writeResponse := request(t, handler, http.MethodPut, "/api/integrations/test/dossier", `{"caseId":"CASE-001","content":"Updated dossier"}`)
	if writeResponse.Code != http.StatusOK || !strings.Contains(writeResponse.Body.String(), "Updated dossier") {
		t.Fatalf("unexpected write response: %d %s", writeResponse.Code, writeResponse.Body.String())
	}
	staleResponse := request(t, handler, http.MethodPut, "/api/integrations/test/dossier", `{"caseId":"CASE-OLD","content":"Stale dossier"}`)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale dossier write = %d %s", staleResponse.Code, staleResponse.Body.String())
	}
}

func newHandler() http.Handler {
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	registry := connectors.NewRegistry(&fakeConnector{})
	return httpapi.New(casefile.NewStore(casefile.BlankCase()), registry, web)
}

type fakeConnector struct {
	content string
}

func (f *fakeConnector) Metadata() connectors.Metadata {
	return connectors.Metadata{ID: "test", Name: "Test", Configured: true, Capabilities: []string{"dossier.read", "dossier.write"}}
}

func (f *fakeConnector) Status(context.Context) connectors.Status {
	return connectors.Status{State: "connected", Message: "ready"}
}

func (f *fakeConnector) ReadDossier(context.Context, connectors.DossierRef) (connectors.Dossier, error) {
	content := f.content
	if content == "" {
		content = "Initial dossier"
	}
	return connectors.Dossier{Content: content, Path: "test.md", Exists: f.content != ""}, nil
}

func (f *fakeConnector) WriteDossier(_ context.Context, _ connectors.DossierRef, write connectors.DossierWrite) (connectors.Dossier, error) {
	f.content = write.Content
	return connectors.Dossier{Content: write.Content, Path: "test.md", Exists: true}, nil
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decode(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
