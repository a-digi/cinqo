package portal

import (
	"errors"
	"strings"
	"unicode/utf8"

	"career-tool-backend/db"
)

// A portal, and each of its links, can carry its own AI platform and model for
// crawling (step 102). Both empty means "no override": the link falls back to its
// portal's choice, and the portal to the tool-wide one (settings.AI). Which
// platforms exist lives in core, so the check that a platform is still there is the
// frontend's; here only the shape is checked. See
// plan/ai/tools/career/step-102-portal-and-link-ai-override.md.

const maxAIOverrideRunes = 200

// ErrInvalidAIOverride is a caller mistake (a 400), never a server failure.
var ErrInvalidAIOverride = errors.New("platformId and model must be plain text of at most 200 characters, and a model needs a platform")

// AIOverride is one level's choice. Both empty removes the override.
type AIOverride struct {
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
}

// cleanAIOverride trims both fields and refuses over-long text, a NUL byte, and a
// model without a platform (a platform with no model is fine: its first model is used).
func cleanAIOverride(in AIOverride) (AIOverride, error) {
	platform, ok1 := cleanAIText(in.PlatformID)
	model, ok2 := cleanAIText(in.Model)
	if !ok1 || !ok2 || (platform == "" && model != "") {
		return AIOverride{}, ErrInvalidAIOverride
	}
	return AIOverride{PlatformID: platform, Model: model}, nil
}

func cleanAIText(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsRune(s, 0) || utf8.RuneCountInString(s) > maxAIOverrideRunes {
		return "", false
	}
	return s, true
}

// SetPortalAI stores (or, with both fields empty, removes) a portal's own AI choice.
func SetPortalAI(id string, in AIOverride) error {
	o, err := cleanAIOverride(in)
	if err != nil {
		return err
	}
	if err := requirePortalExists(id); err != nil {
		return err
	}
	_, err = db.JobsDB.Exec(
		`UPDATE portals SET ai_platform_id = ?, ai_model = ?, updated_at = datetime('now') WHERE id = ?`,
		o.PlatformID, o.Model, id,
	)
	return err
}

// SetLinkAI stores (or, with both fields empty, removes) a link's own AI choice.
func SetLinkAI(id string, in AIOverride) error {
	o, err := cleanAIOverride(in)
	if err != nil {
		return err
	}
	if err := RequirePortalLinkExists(id); err != nil {
		return err
	}
	_, err = db.JobsDB.Exec(
		`UPDATE portal_links SET ai_platform_id = ?, ai_model = ?, updated_at = datetime('now') WHERE id = ?`,
		o.PlatformID, o.Model, id,
	)
	return err
}
