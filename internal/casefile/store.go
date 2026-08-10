package casefile

import (
	"errors"
	"sync"
)

var ErrEventNotFound = errors.New("event not found")

type Case struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner"`
	UpdatedAt   string   `json:"updatedAt"`
	Subject     Subject  `json:"subject"`
	Events      []Event  `json:"events"`
	SourceCount int      `json:"sourceCount"`
	Tags        []string `json:"tags"`
}

type Subject struct {
	Codename    string       `json:"codename"`
	DisplayName string       `json:"displayName"`
	Risk        string       `json:"risk"`
	Confidence  int          `json:"confidence"`
	Location    string       `json:"location"`
	LastSeen    string       `json:"lastSeen"`
	Aliases     []string     `json:"aliases"`
	Identifiers []Identifier `json:"identifiers"`
	Relations   []Relation   `json:"relations"`
}

type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Relation struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Risk string `json:"risk"`
}

type Event struct {
	ID          string   `json:"id"`
	Time        string   `json:"time"`
	Date        string   `json:"date"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Source      string   `json:"source"`
	SourceURL   string   `json:"sourceUrl"`
	Confidence  int      `json:"confidence"`
	Status      string   `json:"status"`
	Fingerprint string   `json:"fingerprint"`
	Indicators  []string `json:"indicators"`
	Notes       []Note   `json:"notes"`
}

type Note struct {
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

type Store struct {
	mu       sync.RWMutex
	caseData Case
}

func NewStore(initial Case) *Store {
	return &Store{caseData: initial}
}

func (s *Store) Snapshot() Case {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCase(s.caseData)
}

func (s *Store) SetStatus(eventID, status string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.caseData.Events {
		if s.caseData.Events[i].ID == eventID {
			s.caseData.Events[i].Status = status
			return cloneEvent(s.caseData.Events[i]), nil
		}
	}
	return Event{}, ErrEventNotFound
}

func (s *Store) AddNote(eventID string, note Note) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.caseData.Events {
		if s.caseData.Events[i].ID == eventID {
			s.caseData.Events[i].Notes = append(s.caseData.Events[i].Notes, note)
			return cloneEvent(s.caseData.Events[i]), nil
		}
	}
	return Event{}, ErrEventNotFound
}

func cloneCase(source Case) Case {
	copy := source
	copy.Tags = append([]string(nil), source.Tags...)
	copy.Subject.Aliases = append([]string(nil), source.Subject.Aliases...)
	copy.Subject.Identifiers = append([]Identifier(nil), source.Subject.Identifiers...)
	copy.Subject.Relations = append([]Relation(nil), source.Subject.Relations...)
	copy.Events = make([]Event, len(source.Events))
	for i, event := range source.Events {
		copy.Events[i] = cloneEvent(event)
	}
	return copy
}

func cloneEvent(source Event) Event {
	copy := source
	copy.Indicators = append([]string(nil), source.Indicators...)
	copy.Notes = append([]Note(nil), source.Notes...)
	return copy
}
