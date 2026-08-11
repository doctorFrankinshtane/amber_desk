package httpapi

import (
	"net/http"

	"amberdesk/internal/connectors"
)

const runtimeConfigVersion = 1

type runtimeConfig struct {
	Version    int `json:"version"`
	Connectors struct {
		ConnectedState string `json:"connectedState"`
		MemoryBackend  string `json:"memoryBackend"`
	} `json:"connectors"`
	Attachments struct {
		MaxBytes         int64    `json:"maxBytes"`
		MaxPerNode       int      `json:"maxPerNode"`
		MaxFilenameRunes int      `json:"maxFilenameRunes"`
		ImageMediaTypes  []string `json:"imageMediaTypes"`
	} `json:"attachments"`
	Timeline struct {
		TitleMaxRunes   int `json:"titleMaxRunes"`
		SummaryMaxRunes int `json:"summaryMaxRunes"`
		SourceMaxRunes  int `json:"sourceMaxRunes"`
		NoteMaxRunes    int `json:"noteMaxRunes"`
	} `json:"timeline"`
}

func currentRuntimeConfig() runtimeConfig {
	config := runtimeConfig{Version: runtimeConfigVersion}
	config.Connectors.ConnectedState = connectors.StateConnected
	config.Connectors.MemoryBackend = connectors.BackendMemory
	config.Attachments.MaxBytes = connectors.MaxRelationshipAttachmentSize
	config.Attachments.MaxPerNode = connectors.MaxRelationshipAttachmentsPerNode
	config.Attachments.MaxFilenameRunes = connectors.MaxRelationshipAttachmentFilenameRunes
	config.Attachments.ImageMediaTypes = connectors.RelationshipImageMediaTypes()
	config.Timeline.TitleMaxRunes = connectors.MaxTimelineTitleRunes
	config.Timeline.SummaryMaxRunes = connectors.MaxTimelineSummaryRunes
	config.Timeline.SourceMaxRunes = connectors.MaxTimelineSourceRunes
	config.Timeline.NoteMaxRunes = connectors.MaxTimelineNoteRunes
	return config
}

func (h *Handler) getRuntimeConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, currentRuntimeConfig())
}
