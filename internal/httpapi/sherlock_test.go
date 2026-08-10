package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/internal/httpapi"
	"amberdesk/internal/sherlock"
	"amberdesk/pkg/connectors"
)

type fakeSherlockRunner struct{}

func (fakeSherlockRunner) Status(context.Context) sherlock.RunnerStatus {
	return sherlock.RunnerStatus{Ready: true, Version: sherlock.SupportedVersion, Message: "ready"}
}

func (fakeSherlockRunner) Run(_ context.Context, request sherlock.ScanRequest, emit sherlock.EventSink) (sherlock.ScanReport, error) {
	emit(sherlock.Event{Type: "progress", Message: "[+] GitHub", Checked: 1, Claimed: 1})
	return sherlock.ScanReport{Username: request.Username, Results: []sherlock.Result{{ID: "github-result", Site: "GitHub", ProfileURL: "https://github.com/handle", MainURL: "https://github.com", Status: "claimed", HTTPStatus: 200}}}, nil
}

func TestSherlockManualImport(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	handler := httpapi.NewWithConfig(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(), web, httpapi.Config{SherlockRunner: fakeSherlockRunner{}, AllowRemoteToolRuns: true})
	created := request(t, handler, http.MethodPost, "/api/case", `{"name":"SHERLOCK CASE","subject":{"codename":"TARGET","risk":"low","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var dossier struct {
		Case          casefile.Case                   `json:"case"`
		Relationships connectors.RelationshipSnapshot `json:"relationships"`
	}
	decode(t, created, &dossier)
	startBody := `{"caseId":"` + dossier.Case.ID + `","sourceNodeId":"` + dossier.Relationships.Nodes[0].ID + `","username":"handle","externalTrafficConfirmed":true}`
	started := request(t, handler, http.MethodPost, "/api/tools/sherlock/scans", startBody)
	if started.Code != http.StatusAccepted {
		t.Fatalf("start scan: %d %s", started.Code, started.Body.String())
	}
	var scan sherlock.Scan
	decode(t, started, &scan)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		response := request(t, handler, http.MethodGet, "/api/tools/sherlock/scans/"+scan.ID, "")
		decode(t, response, &scan)
		if scan.State == "completed" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if scan.State != "completed" || scan.Claimed != 1 {
		t.Fatalf("scan did not complete: %+v", scan)
	}
	imported := request(t, handler, http.MethodPost, "/api/tools/sherlock/scans/"+scan.ID+"/import", `{"caseId":"`+dossier.Case.ID+`","resultIds":["github-result"]}`)
	if imported.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", imported.Code, imported.Body.String())
	}
	graph := relationshipSnapshot(t, handler)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].Label != "FOUND ON" || graph.Nodes[1].AttachmentCount != 0 {
		t.Fatalf("unexpected imported graph: %+v", graph)
	}
	attachments := request(t, handler, http.MethodGet, "/api/relationships/nodes/"+dossier.Relationships.Nodes[0].ID+"/attachments", "")
	if attachments.Code != http.StatusOK || !containsBody(attachments.Body.String(), "sherlock-handle-") {
		t.Fatalf("report attachment: %d %s", attachments.Code, attachments.Body.String())
	}
	timeline := request(t, handler, http.MethodGet, "/api/timeline", "")
	if timeline.Code != http.StatusOK || !containsBody(timeline.Body.String(), "Sherlock scan: handle") {
		t.Fatalf("timeline: %d %s", timeline.Code, timeline.Body.String())
	}
}

func TestSherlockRequiresExplicitConfirmation(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	handler := httpapi.NewWithConfig(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(), web, httpapi.Config{SherlockRunner: fakeSherlockRunner{}, AllowRemoteToolRuns: true})
	response := request(t, handler, http.MethodPost, "/api/tools/sherlock/scans", `{"caseId":"CASE-001","sourceNodeId":"NODE-X","username":"handle","externalTrafficConfirmed":false}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("confirmation status = %d", response.Code)
	}
}

func containsBody(value, expected string) bool {
	return len(value) >= len(expected) && (value == expected || find(value, expected) >= 0)
}

func find(value, expected string) int {
	for index := 0; index+len(expected) <= len(value); index++ {
		if value[index:index+len(expected)] == expected {
			return index
		}
	}
	return -1
}
