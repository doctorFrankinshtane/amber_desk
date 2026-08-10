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
	Content    string `json:"content"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type DossierWrite struct {
	Content            string
	ExpectedModifiedAt string
}

type Connector interface {
	Metadata() Metadata
	Status(context.Context) Status
	ReadDossier(context.Context, DossierRef) (Dossier, error)
	WriteDossier(context.Context, DossierRef, DossierWrite) (Dossier, error)
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
