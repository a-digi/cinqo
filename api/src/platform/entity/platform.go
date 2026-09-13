// Package entity holds the platform system's plain data shapes — see
// plan/ai/platform/step-01-platform-registry-and-data-model.md.
package entity

// Platform is one registered AI provider — a compile-time value, never
// a database row (verified directly against the real reference
// implementation before designing this: adding a provider needs a
// real, reviewed ChatCompleter implementation regardless, so a DB row
// alone could never make an arbitrary new provider actually work).
type Platform struct {
	ID   string `json:"id"`   // "openai" | "anthropic" | "openrouter"
	Name string `json:"name"` // "OpenAI" | "Anthropic" | "OpenRouter"
}
