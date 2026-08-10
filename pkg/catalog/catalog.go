package catalog

import "context"

type Metadata struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Version    string `json:"version"`
	ImportedAt string `json:"importedAt"`
	License    string `json:"license"`
}

type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolCount int    `json:"toolCount"`
}

type Tool struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	Description    string   `json:"description,omitempty"`
	Status         string   `json:"status,omitempty"`
	Pricing        string   `json:"pricing,omitempty"`
	BestFor        string   `json:"bestFor,omitempty"`
	Input          string   `json:"input,omitempty"`
	Output         string   `json:"output,omitempty"`
	OPSEC          string   `json:"opsec,omitempty"`
	OPSECNote      string   `json:"opsecNote,omitempty"`
	LocalInstall   bool     `json:"localInstall"`
	GoogleDork     bool     `json:"googleDork"`
	Registration   bool     `json:"registration"`
	EditableURL    bool     `json:"editableUrl"`
	API            bool     `json:"api"`
	InvitationOnly bool     `json:"invitationOnly"`
	Deprecated     bool     `json:"deprecated"`
	InsecureURL    bool     `json:"insecureUrl"`
	Badges         []string `json:"badges"`
	Path           []string `json:"path"`
}

type Snapshot struct {
	Metadata   Metadata   `json:"metadata"`
	Categories []Category `json:"categories"`
	Tools      []Tool     `json:"tools"`
	Excluded   int        `json:"excluded"`
}

type Provider interface {
	Snapshot(context.Context) (Snapshot, error)
}
