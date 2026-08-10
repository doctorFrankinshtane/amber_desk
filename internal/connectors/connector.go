package connectors

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var (
	ErrNotConfigured   = errors.New("connector is not configured")
	ErrConnectorAbsent = errors.New("connector not found")
	ErrConflict        = errors.New("dossier changed outside Amber Desk")
	ErrEntityAbsent    = errors.New("connector entity not found")
)

type Metadata struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Configured   bool     `json:"configured"`
}

type Status struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type Info struct {
	Metadata Metadata `json:"metadata"`
	Status   Status   `json:"status"`
}

type DossierRef struct {
	CaseID      string
	CaseName    string
	SubjectName string
}

type Dossier struct {
	CaseID     string `json:"caseId"`
	Content    string `json:"content"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type DossierWrite struct {
	Content            string
	ExpectedModifiedAt string
}

type CaseSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
	Active    bool   `json:"active"`
}

type TimelineNote struct {
	Text      string `json:"text" yaml:"text"`
	CreatedAt string `json:"createdAt" yaml:"created_at"`
}

type TimelineEvent struct {
	ID          string         `json:"id" yaml:"id"`
	OccurredAt  string         `json:"occurredAt,omitempty" yaml:"occurred_at,omitempty"`
	Time        string         `json:"time" yaml:"time"`
	Date        string         `json:"date" yaml:"date"`
	Type        string         `json:"type" yaml:"type"`
	Title       string         `json:"title" yaml:"title"`
	Summary     string         `json:"summary" yaml:"summary"`
	Source      string         `json:"source" yaml:"source"`
	SourceURL   string         `json:"sourceUrl" yaml:"source_url"`
	Confidence  int            `json:"confidence" yaml:"confidence"`
	Status      string         `json:"status" yaml:"status"`
	Fingerprint string         `json:"fingerprint" yaml:"fingerprint"`
	Indicators  []string       `json:"indicators" yaml:"indicators"`
	Notes       []TimelineNote `json:"notes" yaml:"notes"`
	Latitude    *float64       `json:"latitude,omitempty" yaml:"latitude,omitempty"`
	Longitude   *float64       `json:"longitude,omitempty" yaml:"longitude,omitempty"`
}

type TimelineSnapshot struct {
	Events  []TimelineEvent `json:"events"`
	Backend string          `json:"backend"`
}

type MapMarker struct {
	ID          string   `json:"id" yaml:"id"`
	Label       string   `json:"label" yaml:"label"`
	Latitude    float64  `json:"latitude" yaml:"latitude"`
	Longitude   float64  `json:"longitude" yaml:"longitude"`
	OccurredAt  string   `json:"occurredAt" yaml:"occurred_at"`
	Description string   `json:"description" yaml:"description"`
	EventIDs    []string `json:"eventIds" yaml:"event_ids"`
}

type MapRoute struct {
	ID           string `json:"id" yaml:"id"`
	FromMarkerID string `json:"fromMarkerId" yaml:"from_marker_id"`
	ToMarkerID   string `json:"toMarkerId" yaml:"to_marker_id"`
	Label        string `json:"label" yaml:"label"`
	StartedAt    string `json:"startedAt" yaml:"started_at"`
	EndedAt      string `json:"endedAt" yaml:"ended_at"`
	Notes        string `json:"notes" yaml:"notes"`
}

type MapSnapshot struct {
	Markers []MapMarker `json:"markers"`
	Routes  []MapRoute  `json:"routes"`
	Backend string      `json:"backend"`
}

type RelationshipNode struct {
	ID        string   `json:"id" yaml:"id"`
	Type      string   `json:"type" yaml:"type"`
	Title     string   `json:"title" yaml:"title"`
	Subtitle  string   `json:"subtitle" yaml:"subtitle"`
	Details   string   `json:"details" yaml:"details"`
	Risk      string   `json:"risk" yaml:"risk"`
	SourceIDs []string `json:"sourceIds" yaml:"source_ids"`
	X         float64  `json:"x" yaml:"x"`
	Y         float64  `json:"y" yaml:"y"`
	Primary   bool     `json:"primary" yaml:"primary"`
}

type RelationshipEdge struct {
	ID         string   `json:"id" yaml:"id"`
	SourceID   string   `json:"sourceId" yaml:"source_id"`
	TargetID   string   `json:"targetId" yaml:"target_id"`
	Label      string   `json:"label" yaml:"label"`
	Confidence int      `json:"confidence" yaml:"confidence"`
	SourceIDs  []string `json:"sourceIds" yaml:"source_ids"`
	Note       string   `json:"note" yaml:"note"`
	Kind       string   `json:"kind" yaml:"kind"`
}

type RelationshipSnapshot struct {
	Nodes   []RelationshipNode `json:"nodes"`
	Edges   []RelationshipEdge `json:"edges"`
	Backend string             `json:"backend"`
}

type Connector interface {
	Metadata() Metadata
	Status(context.Context) Status
	ReadDossier(context.Context, DossierRef) (Dossier, error)
	WriteDossier(context.Context, DossierRef, DossierWrite) (Dossier, error)
}

type TimelineConnector interface {
	Connector
	ListTimeline(context.Context, DossierRef) ([]TimelineEvent, error)
	BootstrapTimeline(context.Context, DossierRef, []TimelineEvent) ([]TimelineEvent, error)
	CreateTimelineEvent(context.Context, DossierRef, TimelineEvent) (TimelineEvent, error)
	SetTimelineStatus(context.Context, DossierRef, string, string) (TimelineEvent, error)
	AddTimelineNote(context.Context, DossierRef, string, TimelineNote) (TimelineEvent, error)
}

type TimelineDeleteConnector interface {
	Connector
	DeleteTimelineEvent(context.Context, DossierRef, string) error
}

type MapConnector interface {
	Connector
	ListMap(context.Context, DossierRef) (MapSnapshot, error)
	CreateMapMarker(context.Context, DossierRef, MapMarker) (MapMarker, error)
	UpdateMapMarker(context.Context, DossierRef, MapMarker) (MapMarker, error)
	DeleteMapMarker(context.Context, DossierRef, string) error
	CreateMapRoute(context.Context, DossierRef, MapRoute) (MapRoute, error)
	DeleteMapRoute(context.Context, DossierRef, string) error
}

type RelationshipConnector interface {
	Connector
	ListRelationships(context.Context, DossierRef) (RelationshipSnapshot, error)
	CreateRelationshipNode(context.Context, DossierRef, RelationshipNode) (RelationshipNode, error)
	UpdateRelationshipNode(context.Context, DossierRef, RelationshipNode) (RelationshipNode, error)
	DeleteRelationshipNode(context.Context, DossierRef, string) error
	CreateRelationshipEdge(context.Context, DossierRef, RelationshipEdge) (RelationshipEdge, error)
	UpdateRelationshipEdge(context.Context, DossierRef, RelationshipEdge) (RelationshipEdge, error)
	DeleteRelationshipEdge(context.Context, DossierRef, string) error
}

// WorkspaceStateConnector persists small opaque core snapshots without coupling
// extension providers to Amber Desk's internal case model.
type WorkspaceStateConnector interface {
	Connector
	ReadWorkspaceState(context.Context, string) ([]byte, error)
	WriteWorkspaceState(context.Context, string, []byte) error
}

// CaseStoreConnector owns case discovery and lifecycle while treating the
// core snapshot as opaque JSON.
type CaseStoreConnector interface {
	Connector
	ListCases(context.Context) ([]CaseSummary, error)
	ReadCase(context.Context, string) ([]byte, error)
	WriteCase(context.Context, CaseSummary, []byte) error
	SetActiveCase(context.Context, string) error
	ActiveCaseID(context.Context) (string, error)
	TrashCase(context.Context, string) (string, error)
}

type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
}

func NewRegistry(items ...Connector) *Registry {
	registry := &Registry{connectors: make(map[string]Connector, len(items))}
	for _, item := range items {
		registry.Register(item)
	}
	return registry
}

func (r *Registry) Register(connector Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[connector.Metadata().ID] = connector
}

func (r *Registry) Get(id string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	connector, ok := r.connectors[id]
	if !ok {
		return nil, ErrConnectorAbsent
	}
	return connector, nil
}

func (r *Registry) List(ctx context.Context) []Info {
	r.mu.RLock()
	items := make([]Connector, 0, len(r.connectors))
	for _, connector := range r.connectors {
		items = append(items, connector)
	}
	r.mu.RUnlock()

	result := make([]Info, 0, len(items))
	for _, connector := range items {
		result = append(result, Info{Metadata: connector.Metadata(), Status: connector.Status(ctx)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Metadata.ID < result[j].Metadata.ID })
	return result
}
