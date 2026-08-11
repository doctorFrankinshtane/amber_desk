package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

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

func TestRequestSecurityPolicy(t *testing.T) {
	handler := newHandler()

	remote := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	remote.Host = "amberdesk.lan"
	remoteResponse := httptest.NewRecorder()
	handler.ServeHTTP(remoteResponse, remote)
	if remoteResponse.Code != http.StatusForbidden {
		t.Fatalf("remote Host status = %d, want %d", remoteResponse.Code, http.StatusForbidden)
	}

	foreignOrigin := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	foreignOrigin.Host = "localhost"
	foreignOrigin.Header.Set("Origin", "https://example.test")
	foreignResponse := httptest.NewRecorder()
	handler.ServeHTTP(foreignResponse, foreignOrigin)
	if foreignResponse.Code != http.StatusForbidden {
		t.Fatalf("foreign Origin status = %d, want %d", foreignResponse.Code, http.StatusForbidden)
	}

	crossSite := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	crossSite.Host = "localhost"
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	crossSiteResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossSiteResponse, crossSite)
	if crossSiteResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-site status = %d, want %d", crossSiteResponse.Code, http.StatusForbidden)
	}

	plainText := httptest.NewRequest(http.MethodPost, "/api/timeline/events", strings.NewReader(`{"title":"unsafe"}`))
	plainText.Host = "localhost"
	plainText.Header.Set("Content-Type", "text/plain")
	plainTextResponse := httptest.NewRecorder()
	handler.ServeHTTP(plainTextResponse, plainText)
	if plainTextResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("plain text status = %d, want %d", plainTextResponse.Code, http.StatusUnsupportedMediaType)
	}
}

func TestRemoteAccessRequiresExplicitOptIn(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.NewWithConfig(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(), web, httpapi.Config{AllowRemoteAccess: true})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "amberdesk.lan"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("explicit remote access status = %d: %s", response.Code, response.Body.String())
	}
}

func TestResponseCachePoliciesAndStaticETag(t *testing.T) {
	handler := newHandler()
	apiResponse := request(t, handler, http.MethodGet, "/api/health", "")
	if got := apiResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("API Cache-Control = %q", got)
	}
	if got := apiResponse.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}

	page := request(t, handler, http.MethodGet, "/", "")
	etag := page.Header().Get("ETag")
	if etag == "" || page.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("static cache headers = ETag %q, Cache-Control %q", etag, page.Header().Get("Cache-Control"))
	}
	conditional := httptest.NewRequest(http.MethodGet, "/", nil)
	conditional.Host = "localhost"
	conditional.Header.Set("If-None-Match", etag)
	conditionalResponse := httptest.NewRecorder()
	handler.ServeHTTP(conditionalResponse, conditional)
	if conditionalResponse.Code != http.StatusNotModified || conditionalResponse.Body.Len() != 0 {
		t.Fatalf("conditional response = %d, body %q", conditionalResponse.Code, conditionalResponse.Body.String())
	}
}

func TestStrictJSONLimits(t *testing.T) {
	handler := newHandler()
	trailing := request(t, handler, http.MethodPost, "/api/case", `{"name":"first"}{"name":"second"}`)
	if trailing.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d, want %d", trailing.Code, http.StatusBadRequest)
	}

	oversizedGeneric := request(t, handler, http.MethodPost, "/api/timeline/events", `{"title":"`+strings.Repeat("a", 70<<10)+`"}`)
	if oversizedGeneric.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized generic status = %d, want %d", oversizedGeneric.Code, http.StatusRequestEntityTooLarge)
	}

	largeDossier, err := json.Marshal(map[string]string{
		"caseId":  casefile.BlankCase().ID,
		"content": strings.Repeat("d", 128<<10),
	})
	if err != nil {
		t.Fatal(err)
	}
	dossierResponse := request(t, handler, http.MethodPut, "/api/integrations/test/dossier", string(largeDossier))
	if dossierResponse.Code != http.StatusOK {
		t.Fatalf("large dossier status = %d: %s", dossierResponse.Code, dossierResponse.Body.String())
	}
}

func TestMemoryTimelinePreservesOccurredAt(t *testing.T) {
	handler := newHandler()
	const occurredAt = "2024-02-29T18:37:15+05:00"
	created := request(t, handler, http.MethodPost, "/api/timeline/events", `{"occurredAt":"`+occurredAt+`","title":"Verified sighting","confidence":82}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create timeline event: %d %s", created.Code, created.Body.String())
	}
	var event connectors.TimelineEvent
	decode(t, created, &event)
	if event.OccurredAt != occurredAt {
		t.Fatalf("created occurredAt = %q, want %q", event.OccurredAt, occurredAt)
	}

	listed := request(t, handler, http.MethodGet, "/api/timeline", "")
	var snapshot connectors.TimelineSnapshot
	decode(t, listed, &snapshot)
	if len(snapshot.Events) != 1 || snapshot.Events[0].OccurredAt != occurredAt {
		t.Fatalf("listed timeline = %+v", snapshot.Events)
	}
}

func TestCaseTextLimitsPreserveUTF8(t *testing.T) {
	handler := newHandler()
	name := strings.Repeat("Я", 121)
	body, err := json.Marshal(map[string]any{
		"name": name,
		"subject": map[string]any{
			"codename": "СУБЪЕКТ", "risk": "low", "confidence": 50,
			"aliases": []string{}, "identifiers": []any{}, "relations": []any{},
		},
		"tags": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := request(t, handler, http.MethodPost, "/api/case", string(body))
	if created.Code != http.StatusCreated {
		t.Fatalf("create UTF-8 case: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		Case casefile.Case `json:"case"`
	}
	decode(t, created, &response)
	if !utf8.ValidString(response.Case.Name) || utf8.RuneCountInString(response.Case.Name) != 120 {
		t.Fatalf("limited case name is invalid: %q", response.Case.Name)
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
	second := request(t, handler, http.MethodPost, "/api/case", body)
	if second.Code != http.StatusCreated {
		t.Fatalf("second dossier status = %d", second.Code)
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
	nodes, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Cases", "CASE-*", "Relations", "Nodes", "*.md"))
	edges, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Cases", "CASE-*", "Relations", "Edges", "*.md"))
	if len(dossiers) != 1 || len(nodes) != 2 || len(edges) != 1 {
		t.Fatalf("synced files: dossiers=%v nodes=%v edges=%v", dossiers, nodes, edges)
	}
	caseStore := any(provider).(connectors.CaseStoreConnector)
	activeID, err := caseStore.ActiveCaseID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state, err := caseStore.ReadCase(context.Background(), activeID)
	if err != nil || !strings.Contains(string(state), `"name": "CASE ORION"`) {
		t.Fatalf("persisted active case: %v %s", err, state)
	}
}

func TestCreateDossierRollsBackFailedPersistence(t *testing.T) {
	connector := &failingCaseConnector{}
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(connector), web)
	created := request(t, handler, http.MethodPost, "/api/case", `{"name":"ROLLBACK","subject":{"codename":"TARGET","risk":"low","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	if created.Code != http.StatusBadGateway {
		t.Fatalf("failed persistence status = %d: %s", created.Code, created.Body.String())
	}
	if !connector.written || !connector.trashed {
		t.Fatalf("persistence written=%t trashed=%t", connector.written, connector.trashed)
	}
	current := request(t, handler, http.MethodGet, "/api/case", "")
	var active casefile.Case
	decode(t, current, &active)
	if active.ID != casefile.BlankCase().ID || active.Subject.Codename != "UNASSIGNED" {
		t.Fatalf("active case changed after rollback: %+v", active)
	}
}

func TestObsidianCaseSwitchAndTrashWorkflow(t *testing.T) {
	vault := t.TempDir()
	provider, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)
	firstResponse := request(t, handler, http.MethodPost, "/api/case", `{"name":"FIRST","subject":{"codename":"ALPHA","risk":"low","confidence":60,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var first struct {
		Case casefile.Case `json:"case"`
	}
	decode(t, firstResponse, &first)
	eventResponse := request(t, handler, http.MethodPost, "/api/timeline/events", `{"title":"First event","type":"identity","confidence":70}`)
	var event connectors.TimelineEvent
	decode(t, eventResponse, &event)
	secondResponse := request(t, handler, http.MethodPost, "/api/case", `{"name":"SECOND","subject":{"codename":"BRAVO","risk":"medium","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var second struct {
		Case casefile.Case `json:"case"`
	}
	decode(t, secondResponse, &second)

	list := request(t, handler, http.MethodGet, "/api/cases", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), first.Case.ID) || !strings.Contains(list.Body.String(), second.Case.ID) {
		t.Fatalf("case list: %d %s", list.Code, list.Body.String())
	}
	switchBody := `{"caseId":"` + first.Case.ID + `","expectedCaseId":"` + second.Case.ID + `"}`
	switched := request(t, handler, http.MethodPut, "/api/cases/active", switchBody)
	if switched.Code != http.StatusOK || !strings.Contains(switched.Body.String(), `"name":"FIRST"`) {
		t.Fatalf("switch case: %d %s", switched.Code, switched.Body.String())
	}
	deletedEvent := request(t, handler, http.MethodDelete, "/api/events/"+event.ID, `{"caseId":"`+first.Case.ID+`"}`)
	if deletedEvent.Code != http.StatusOK {
		t.Fatalf("delete event: %d %s", deletedEvent.Code, deletedEvent.Body.String())
	}
	deleteBody := `{"confirmCaseId":"` + first.Case.ID + `","expectedCaseId":"` + first.Case.ID + `"}`
	deletedCase := request(t, handler, http.MethodDelete, "/api/cases/"+first.Case.ID, deleteBody)
	if deletedCase.Code != http.StatusOK || !strings.Contains(deletedCase.Body.String(), second.Case.ID) {
		t.Fatalf("delete case: %d %s", deletedCase.Code, deletedCase.Body.String())
	}
	trash, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", ".trash", first.Case.ID+"-*"))
	if len(trash) != 1 {
		t.Fatalf("case trash entries = %v", trash)
	}
	stale := request(t, handler, http.MethodPut, "/api/cases/active", `{"caseId":"`+second.Case.ID+`","expectedCaseId":"`+first.Case.ID+`"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale switch = %d %s", stale.Code, stale.Body.String())
	}
	deleteLast := `{"confirmCaseId":"` + second.Case.ID + `","expectedCaseId":"` + second.Case.ID + `"}`
	deletedLast := request(t, handler, http.MethodDelete, "/api/cases/"+second.Case.ID, deleteLast)
	if deletedLast.Code != http.StatusOK || !strings.Contains(deletedLast.Body.String(), `"codename":"UNASSIGNED"`) {
		t.Fatalf("delete final case: %d %s", deletedLast.Code, deletedLast.Body.String())
	}
	if _, err := provider.ActiveCaseID(context.Background()); !errors.Is(err, connectors.ErrEntityAbsent) {
		t.Fatalf("active case after final delete = %v", err)
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

func TestRelationshipAttachmentAPIWithObsidian(t *testing.T) {
	vault := t.TempDir()
	provider, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)
	created := request(t, handler, http.MethodPost, "/api/case", `{"name":"ATTACHMENTS","subject":{"codename":"TARGET","risk":"low","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var dossier struct {
		Case          casefile.Case                   `json:"case"`
		Relationships connectors.RelationshipSnapshot `json:"relationships"`
	}
	decode(t, created, &dossier)
	nodeID := dossier.Relationships.Nodes[0].ID
	content := []byte("evidence payload")
	upload := multipartRequest(t, handler, "/api/relationships/nodes/"+nodeID+"/attachments", dossier.Case.ID, "evidence.txt", content)
	if upload.Code != http.StatusCreated {
		t.Fatalf("attachment upload: %d %s", upload.Code, upload.Body.String())
	}
	var attachment connectors.RelationshipAttachment
	decode(t, upload, &attachment)
	sum := sha256.Sum256(content)
	if attachment.Size != int64(len(content)) || attachment.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("attachment response: %+v", attachment)
	}
	list := request(t, handler, http.MethodGet, "/api/relationships/nodes/"+nodeID+"/attachments", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "evidence.txt") {
		t.Fatalf("attachment list: %d %s", list.Code, list.Body.String())
	}
	download := request(t, handler, http.MethodGet, "/api/relationships/nodes/"+nodeID+"/attachments/"+attachment.ID, "")
	if download.Code != http.StatusOK || download.Body.String() != string(content) || download.Header().Get("X-Content-SHA256") != attachment.SHA256 {
		t.Fatalf("attachment download: %d %q %v", download.Code, download.Body.String(), download.Header())
	}
	stale := request(t, handler, http.MethodDelete, "/api/relationships/nodes/"+nodeID+"/attachments/"+attachment.ID, `{"caseId":"CASE-STALE"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale attachment delete: %d %s", stale.Code, stale.Body.String())
	}
	removed := request(t, handler, http.MethodDelete, "/api/relationships/nodes/"+nodeID+"/attachments/"+attachment.ID, `{"caseId":"`+dossier.Case.ID+`"}`)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("attachment delete: %d %s", removed.Code, removed.Body.String())
	}
	large := multipartRequest(t, handler, "/api/relationships/nodes/"+nodeID+"/attachments", dossier.Case.ID, "large.bin", make([]byte, connectors.MaxRelationshipAttachmentSize+1))
	if large.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large attachment: %d %s", large.Code, large.Body.String())
	}
}

func TestRelationshipPhotoCoverWorkflow(t *testing.T) {
	vault := t.TempDir()
	provider, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("Amber Desk")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)
	created := request(t, handler, http.MethodPost, "/api/case", `{"name":"PHOTO COVER","subject":{"codename":"TARGET","risk":"low","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var dossier struct {
		Case          casefile.Case                   `json:"case"`
		Relationships connectors.RelationshipSnapshot `json:"relationships"`
	}
	decode(t, created, &dossier)
	nodeID, caseID := dossier.Relationships.Nodes[0].ID, dossier.Case.ID
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	textAttachment := uploadAttachment(t, handler, nodeID, caseID, "notes.txt", []byte("not an image"))
	brokenImage := multipartRequest(t, handler, "/api/relationships/nodes/"+nodeID+"/attachments", caseID, "broken.png", append([]byte("\x89PNG\r\n\x1a\n"), []byte("broken")...))
	if brokenImage.Code != http.StatusBadRequest {
		t.Fatalf("broken image upload: %d %s", brokenImage.Code, brokenImage.Body.String())
	}
	first := uploadAttachment(t, handler, nodeID, caseID, "first.png", png)
	snapshot := relationshipSnapshot(t, handler)
	if snapshot.Nodes[0].CoverAttachmentID != first.ID {
		t.Fatalf("first image was not assigned as cover: %+v", snapshot.Nodes[0])
	}
	second := uploadAttachment(t, handler, nodeID, caseID, "second.png", png)
	snapshot = relationshipSnapshot(t, handler)
	if snapshot.Nodes[0].CoverAttachmentID != first.ID {
		t.Fatalf("second image replaced the cover: %+v", snapshot.Nodes[0])
	}
	setSecond := request(t, handler, http.MethodPut, "/api/relationships/nodes/"+nodeID+"/cover", `{"caseId":"`+caseID+`","attachmentId":"`+second.ID+`"}`)
	if setSecond.Code != http.StatusOK || !strings.Contains(setSecond.Body.String(), `"coverAttachmentId":"`+second.ID+`"`) {
		t.Fatalf("set second cover: %d %s", setSecond.Code, setSecond.Body.String())
	}
	tamperedNode := relationshipSnapshot(t, handler).Nodes[0]
	tamperedNode.CoverAttachmentID = textAttachment.ID
	tamperedBody, err := json.Marshal(tamperedNode)
	if err != nil {
		t.Fatal(err)
	}
	tampered := request(t, handler, http.MethodPut, "/api/relationships/nodes/"+nodeID, string(tamperedBody))
	if tampered.Code != http.StatusOK || !strings.Contains(tampered.Body.String(), `"coverAttachmentId":"`+second.ID+`"`) {
		t.Fatalf("generic node update changed cover: %d %s", tampered.Code, tampered.Body.String())
	}
	nonImage := request(t, handler, http.MethodPut, "/api/relationships/nodes/"+nodeID+"/cover", `{"caseId":"`+caseID+`","attachmentId":"`+textAttachment.ID+`"}`)
	if nonImage.Code != http.StatusBadRequest {
		t.Fatalf("non-image cover: %d %s", nonImage.Code, nonImage.Body.String())
	}
	stale := request(t, handler, http.MethodPut, "/api/relationships/nodes/"+nodeID+"/cover", `{"caseId":"CASE-STALE","attachmentId":"`+first.ID+`"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale cover: %d %s", stale.Code, stale.Body.String())
	}
	notePath := filepath.Join(vault, "Amber Desk", "Cases", caseID, "Relations", "Nodes", nodeID+".md")
	note, err := os.ReadFile(notePath)
	if err != nil || !strings.Contains(string(note), "cover_attachment_id: "+second.ID) {
		t.Fatalf("cover frontmatter: %v %s", err, note)
	}
	removeSecond := request(t, handler, http.MethodDelete, "/api/relationships/nodes/"+nodeID+"/attachments/"+second.ID, `{"caseId":"`+caseID+`"}`)
	if removeSecond.Code != http.StatusNoContent || relationshipSnapshot(t, handler).Nodes[0].CoverAttachmentID != first.ID {
		t.Fatalf("cover fallback after delete: %d %s", removeSecond.Code, removeSecond.Body.String())
	}
	removeFirst := request(t, handler, http.MethodDelete, "/api/relationships/nodes/"+nodeID+"/attachments/"+first.ID, `{"caseId":"`+caseID+`"}`)
	if removeFirst.Code != http.StatusNoContent || relationshipSnapshot(t, handler).Nodes[0].CoverAttachmentID != "" {
		t.Fatalf("cover clear after final image: %d %s", removeFirst.Code, removeFirst.Body.String())
	}
}

func TestRelationshipPhotoCoverMemoryFallback(t *testing.T) {
	handler := newHandler()
	created := request(t, handler, http.MethodPost, "/api/case", `{"name":"MEMORY PHOTO","subject":{"codename":"TARGET","risk":"low","confidence":50,"aliases":[],"identifiers":[],"relations":[]},"tags":[]}`)
	var dossier struct {
		Case          casefile.Case                   `json:"case"`
		Relationships connectors.RelationshipSnapshot `json:"relationships"`
	}
	decode(t, created, &dossier)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	attachment := uploadAttachment(t, handler, dossier.Relationships.Nodes[0].ID, dossier.Case.ID, "memory.png", png)
	node := relationshipSnapshot(t, handler).Nodes[0]
	if node.CoverAttachmentID != attachment.ID || node.AttachmentCount != 1 {
		t.Fatalf("memory cover: %+v", node)
	}
}

func TestChecklistMemoryWorkflow(t *testing.T) {
	handler := newHandler()
	caseResponse := request(t, handler, http.MethodGet, "/api/case", "")
	var active casefile.Case
	decode(t, caseResponse, &active)

	listed := request(t, handler, http.MethodGet, "/api/checklist", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list checklist: %d %s", listed.Code, listed.Body.String())
	}
	var snapshot connectors.ChecklistSnapshot
	decode(t, listed, &snapshot)
	if snapshot.Backend != "memory" || len(snapshot.Phases) != 8 || len(snapshot.Phases[0].Tasks) == 0 {
		t.Fatalf("default checklist: %+v", snapshot)
	}
	builtInID := snapshot.Phases[0].Tasks[0].ID

	updated := request(t, handler, http.MethodPut, "/api/checklist/tasks/"+builtInID, `{"caseId":"`+active.ID+`","status":"done","recommended":true}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update checklist: %d %s", updated.Code, updated.Body.String())
	}
	decode(t, updated, &snapshot)
	if snapshot.RecommendedTaskID != builtInID || snapshot.Phases[0].Tasks[0].Status != "done" {
		t.Fatalf("updated checklist: %+v", snapshot)
	}

	created := request(t, handler, http.MethodPost, "/api/checklist/tasks", `{"caseId":"`+active.ID+`","phaseId":"scope","title":"Check local archive","note":"Do not upload case data"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create checklist task: %d %s", created.Code, created.Body.String())
	}
	decode(t, created, &snapshot)
	custom := snapshot.Phases[0].Tasks[len(snapshot.Phases[0].Tasks)-1]
	if !custom.Custom || custom.Title != "Check local archive" {
		t.Fatalf("custom task: %+v", custom)
	}

	blocked := request(t, handler, http.MethodDelete, "/api/checklist/tasks/"+builtInID, `{"caseId":"`+active.ID+`"}`)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("delete built-in = %d, want conflict", blocked.Code)
	}
	deleted := request(t, handler, http.MethodDelete, "/api/checklist/tasks/"+custom.ID, `{"caseId":"`+active.ID+`"}`)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete custom = %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestChecklistBootstrapsThroughObsidian(t *testing.T) {
	vault := t.TempDir()
	provider, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<title>Amber Desk</title>")}}
	handler := httpapi.New(casefile.NewStore(casefile.BlankCase()), connectors.NewRegistry(provider), web)
	response := request(t, handler, http.MethodGet, "/api/checklist", "")
	if response.Code != http.StatusOK {
		t.Fatalf("obsidian checklist: %d %s", response.Code, response.Body.String())
	}
	var snapshot connectors.ChecklistSnapshot
	decode(t, response, &snapshot)
	if snapshot.Backend != "obsidian" || len(snapshot.Phases) != 8 {
		t.Fatalf("obsidian snapshot: %+v", snapshot)
	}
	path := filepath.Join(vault, "Amber Desk", "Cases", casefile.BlankCase().ID, "Checklist.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checklist was not bootstrapped: %v", err)
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

type failingCaseConnector struct {
	written bool
	trashed bool
}

func (f *failingCaseConnector) Metadata() connectors.Metadata {
	return connectors.Metadata{ID: "failing-cases", Name: "Failing cases", Configured: true, Capabilities: []string{"dossier.write", "cases.read", "cases.write", "cases.delete"}}
}

func (f *failingCaseConnector) Status(context.Context) connectors.Status {
	return connectors.Status{State: "connected", Message: "test"}
}

func (f *failingCaseConnector) ReadDossier(context.Context, connectors.DossierRef) (connectors.Dossier, error) {
	return connectors.Dossier{}, connectors.ErrEntityAbsent
}

func (f *failingCaseConnector) WriteDossier(context.Context, connectors.DossierRef, connectors.DossierWrite) (connectors.Dossier, error) {
	return connectors.Dossier{}, errors.New("injected dossier failure")
}

func (f *failingCaseConnector) ListCases(context.Context) ([]connectors.CaseSummary, error) {
	return []connectors.CaseSummary{}, nil
}

func (f *failingCaseConnector) ReadCase(context.Context, string) ([]byte, error) {
	return nil, connectors.ErrEntityAbsent
}

func (f *failingCaseConnector) WriteCase(context.Context, connectors.CaseSummary, []byte) error {
	f.written = true
	return nil
}

func (f *failingCaseConnector) SetActiveCase(context.Context, string) error { return nil }

func (f *failingCaseConnector) ActiveCaseID(context.Context) (string, error) {
	return "", connectors.ErrEntityAbsent
}

func (f *failingCaseConnector) TrashCase(context.Context, string) (string, error) {
	f.trashed = true
	return "", nil
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
	req.Host = "localhost"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func multipartRequest(t *testing.T, handler http.Handler, path, caseID, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("caseId", caseID); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Host = "localhost"
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func uploadAttachment(t *testing.T, handler http.Handler, nodeID, caseID, filename string, content []byte) connectors.RelationshipAttachment {
	t.Helper()
	response := multipartRequest(t, handler, "/api/relationships/nodes/"+nodeID+"/attachments", caseID, filename, content)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload %s: %d %s", filename, response.Code, response.Body.String())
	}
	var attachment connectors.RelationshipAttachment
	decode(t, response, &attachment)
	return attachment
}

func relationshipSnapshot(t *testing.T, handler http.Handler) connectors.RelationshipSnapshot {
	t.Helper()
	response := request(t, handler, http.MethodGet, "/api/relationships", "")
	if response.Code != http.StatusOK {
		t.Fatalf("relationship snapshot: %d %s", response.Code, response.Body.String())
	}
	var snapshot connectors.RelationshipSnapshot
	decode(t, response, &snapshot)
	return snapshot
}

func decode(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
