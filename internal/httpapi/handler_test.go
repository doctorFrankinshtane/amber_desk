package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if caseData.Name != "NORTHSTAR" || len(caseData.Events) == 0 {
		t.Fatalf("unexpected case response: %+v", caseData)
	}

	statusResponse := request(t, handler, http.MethodPatch, "/api/events/EV-107/status", `{"status":"verified"}`)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var updated casefile.Event
	decode(t, statusResponse, &updated)
	if updated.Status != "verified" {
		t.Fatalf("event status = %q", updated.Status)
	}

	noteResponse := request(t, handler, http.MethodPost, "/api/events/EV-107/notes", `{"text":"Independent confirmation"}`)
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
	handler := httpapi.New(casefile.NewStore(casefile.DemoCase()), connectors.NewRegistry(provider), web)

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

	writeResponse := request(t, handler, http.MethodPut, "/api/integrations/test/dossier", `{"content":"Updated dossier"}`)
	if writeResponse.Code != http.StatusOK || !strings.Contains(writeResponse.Body.String(), "Updated dossier") {
		t.Fatalf("unexpected write response: %d %s", writeResponse.Code, writeResponse.Body.String())
	}
}

func newHandler() http.Handler {
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	registry := connectors.NewRegistry(&fakeConnector{})
	return httpapi.New(casefile.NewStore(casefile.DemoCase()), registry, web)
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
