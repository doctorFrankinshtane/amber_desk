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
