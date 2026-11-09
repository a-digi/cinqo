package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/localai/catalog"
)

// Statuses of an added model. A suggestion is only a row: nothing is downloaded and the model is not part of the list of models until it is
// confirmed (added). A dismissed suggestion is kept so a tool does not offer the same file again.
const (
	AddedStatusSuggested = "suggested"
	AddedStatusAdded     = "added"
	AddedStatusDismissed = "dismissed"
)

// AddedIDPrefix starts the id of every added model: no shipped model id may start with it, so an added model can never take a shipped one's place.
const AddedIDPrefix = "added-"

// DefaultAddedContextTokens is the context size assumed for an added model whose suggestion states none.
const DefaultAddedContextTokens = 4096

// AddedModel is one model that was suggested by a tool (plan/ai/small-llm-local/step-220-tools-suggest-local-models.md). Everything about the file
// (the pinned commit, the size, the SHA-256, the licence) was looked up by Cinqo itself, never taken from the tool.
type AddedModel struct {
	_             struct{} `table:"local_ai_models"`
	ID            string   `db:"id" dbtype:"TEXT" nullable:"false" json:"id"`
	Repo          string   `db:"repo" dbtype:"TEXT" nullable:"false" json:"repo"`
	File          string   `db:"file" dbtype:"TEXT" nullable:"false" json:"file"`
	Revision      string   `db:"revision" dbtype:"TEXT" nullable:"false" json:"revision"`
	URL           string   `db:"url" dbtype:"TEXT" nullable:"false" json:"url"`
	SHA256        string   `db:"sha256" dbtype:"TEXT" nullable:"false" json:"sha256"`
	SizeBytes     int64    `db:"size_bytes" dbtype:"INTEGER" nullable:"false" json:"sizeBytes"`
	Quantization  string   `db:"quantization" dbtype:"TEXT" nullable:"false" json:"quantization"`
	License       string   `db:"license" dbtype:"TEXT" nullable:"false" json:"license"`
	Name          string   `db:"name" dbtype:"TEXT" nullable:"false" json:"name"`
	Publisher     string   `db:"publisher" dbtype:"TEXT" nullable:"false" json:"publisher"`
	ContextTokens int      `db:"context_tokens" dbtype:"INTEGER" nullable:"false" json:"contextTokens"`
	Why           string   `db:"why" dbtype:"TEXT" nullable:"false" json:"why"`
	SuggestedBy   string   `db:"suggested_by" dbtype:"TEXT" nullable:"false" json:"suggestedBy"`
	Status        string   `db:"status" dbtype:"TEXT" nullable:"false" json:"status"`
	CreatedAt     string   `db:"created_at" dbtype:"TEXT" nullable:"false" json:"createdAt"`
	UpdatedAt     *string  `db:"updated_at" dbtype:"TEXT" nullable:"true" json:"-"`
}

// CatalogModel is the entry this model has in the list of models. A tool's claims about what a model can do are not trusted: it has no tools, no
// dialect, no driver and no unsuitable-for list (those are only ever written for a model we measured), so a conversation on it only answers. The memory
// check does not count its context cache (kvBytesPerToken 0 means "not stated").
func (a AddedModel) CatalogModel() catalog.Model {
	quant := strings.TrimSpace(a.Quantization)
	if quant == "" {
		quant = "unknown"
	}
	ctx := a.ContextTokens
	if ctx == 0 {
		ctx = DefaultAddedContextTokens
	}
	source := fmt.Sprintf("%s at commit %s (added by %s)", a.Repo, shortRevision(a.Revision), orUnknown(a.SuggestedBy))
	if len(source) > 200 {
		source = source[:200]
	}
	note := strings.TrimSpace(a.Why)
	if len(note) > 300 {
		note = note[:300]
	}
	checked := a.CreatedAt
	if len(checked) >= 10 {
		checked = checked[:10]
	}
	return catalog.Model{
		ID: a.ID, Name: a.Name, Quantization: quant, SizeBytes: a.SizeBytes, SHA256: a.SHA256, URL: a.URL, License: a.License,
		ContextTokens: ctx, Tools: false, Source: source, Note: note, CheckedOn: checked,
	}
}

func shortRevision(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "a tool"
	}
	return strings.TrimSpace(s)
}

// AddedID is the id of an added model: the prefix and the first 12 hex digits of a hash of repository, file and pinned commit, so the same file at the
// same commit always has the same id and a different commit of it has another.
func AddedID(repo, file, commit string) string {
	sum := sha256.Sum256([]byte(repo + "\n" + file + "\n" + commit))
	return AddedIDPrefix + hex.EncodeToString(sum[:])[:12]
}
