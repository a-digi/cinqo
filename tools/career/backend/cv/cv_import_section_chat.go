// cv_import_section_chat.go — a small chat about one section (or some rows) of a CV import's suggestions (plan 268). The review page opens a core conversation for it and records here
// which run, which section and which rows the chat is for. The two suggestion tools learn which conversation is calling from TOOL_CONVERSATION_ID (set by the core, never by the
// model) and, when that conversation has a binding, keep the AI inside it: the read shows only the bound part and a change outside it is refused as a whole. A conversation without a
// binding (the import's own chat, an ordinary chat) behaves exactly as before. Nothing here inserts anything into the profile.
package cv

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// MaxSectionChatRows is how many rows one chat may be bound to.
const MaxSectionChatRows = 100

// conversationIDPattern is the shape of a conversation id the core puts into a tool's environment; anything else means "no conversation".
var conversationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

var (
	errChatAlreadyBound = errors.New("this conversation is already bound to a section")
	errChatRunClosed    = errors.New("this import is not open for changes: it is still being analysed, failed, or was already saved from or declined")
)

// callerConversation is the conversation the current tool call belongs to, from the environment the core sets; ” when there is none or it is not a plain id.
func callerConversation() string {
	id := strings.TrimSpace(os.Getenv("TOOL_CONVERSATION_ID"))
	if !conversationIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// sectionChat is what a conversation is bound to.
type sectionChat struct {
	ID             string
	RunID          string
	ConversationID string
	Section        string
	ItemIDs        []string
}

func (c sectionChat) isList() bool { return contains(lists, c.Section) }

// describe says what the chat is about, for the AI and for refusals: "the Skills section", "2 rows of the Experience section".
func (c sectionChat) describe() string {
	name := sectionName(c.Section)
	if len(c.ItemIDs) == 0 {
		return "the " + name + " section"
	}
	if len(c.ItemIDs) == 1 {
		return "one row of the " + name + " section"
	}
	return fmt.Sprintf("%d rows of the %s section", len(c.ItemIDs), name)
}

func sectionName(section string) string {
	switch section {
	case "persona":
		return "Target roles"
	case "personalDetails":
		return "Personal Details"
	case "profile", "skills", "experience", "education":
		return strings.ToUpper(section[:1]) + section[1:]
	}
	return section
}

// RecordSectionChat binds a conversation to a section (and some rows of a list) of a run that is open for changes, and returns the binding's id. It is refused for a run that is not
// ready or already has a save summary, an unknown section, row ids for a single section or ids that are not rows of the list, and for a conversation that is already bound.
func RecordSectionChat(runID, conversationID, section string, itemIDs []string) (string, error) {
	runID, conversationID, section = strings.TrimSpace(runID), strings.TrimSpace(conversationID), strings.TrimSpace(section)
	if !conversationIDPattern.MatchString(conversationID) {
		return "", problem("conversationId must be the id of the chat (letters, digits, - and _)")
	}
	if !contains(sections, section) && !contains(lists, section) {
		return "", problem("section must be one of %s, %s", strings.Join(sections, ", "), strings.Join(lists, ", "))
	}
	ids := make([]string, 0, len(itemIDs))
	seen := map[string]bool{}
	for _, id := range itemIDs {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 && !contains(lists, section) {
		return "", problem("%s is a single section: itemIds only go with skills, experience or education", section)
	}
	if len(ids) > MaxSectionChatRows {
		return "", problem("a chat can be bound to at most %d rows (itemIds has %d)", MaxSectionChatRows, len(ids))
	}
	run, p, _, err := readyProposal(runID)
	if err != nil {
		return "", err
	}
	if !noRawJSON(run.SaveSummary) {
		return "", errChatRunClosed
	}
	if len(ids) > 0 {
		have := map[string]bool{}
		switch section {
		case "skills":
			for _, r := range p.Skills {
				have[r.Id] = true
			}
		case "experience":
			for _, r := range p.Experience {
				have[r.Id] = true
			}
		default:
			for _, r := range p.Education {
				have[r.Id] = true
			}
		}
		for _, id := range ids {
			if !have[id] {
				return "", problem("there is no row with id %q in %s", id, section)
			}
		}
	}
	stored := ""
	if len(ids) > 0 {
		raw, err := json.Marshal(ids)
		if err != nil {
			return "", err
		}
		stored = string(raw)
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(`INSERT INTO cv_import_section_chats (id, run_id, conversation_id, section, item_ids) VALUES (?, ?, ?, ?, ?)`, id, run.ID, conversationID, section, stored); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: cv_import_section_chats.conversation_id") {
			return "", errChatAlreadyBound
		}
		return "", err
	}
	return id, nil
}

// sectionChatOf is the binding of a conversation, or nil when it has none (and for an empty conversation id).
func sectionChatOf(conversationID string) (*sectionChat, error) {
	if conversationID == "" {
		return nil, nil
	}
	var c sectionChat
	var raw string
	err := db.CareerDB.QueryRow(`SELECT id, run_id, conversation_id, section, item_ids FROM cv_import_section_chats WHERE conversation_id = ?`, conversationID).Scan(&c.ID, &c.RunID, &c.ConversationID, &c.Section, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.ItemIDs); err != nil {
			return nil, fmt.Errorf("the chat's binding is unreadable: %w", err)
		}
	}
	return &c, nil
}

func (c sectionChat) holdsRow(id string) bool {
	if len(c.ItemIDs) == 0 {
		return true
	}
	return contains(c.ItemIDs, id)
}

// checkChangesInScope refuses the whole call when any operation reaches outside the chat's section or rows; the first offender is named. An operation is inside when it updates the bound
// single section, or (for a bound list) updates or removes a row of that list that the chat holds, or adds a row to it when the chat is about the whole list.
func (c sectionChat) checkChangesInScope(changes []SuggestionChange) error {
	for i, ch := range changes {
		path := fmt.Sprintf("changes[%d]", i)
		target := ch.Section
		if target == "" {
			target = ch.List
		}
		if target != c.Section {
			if target == "" {
				continue // not a well formed operation: the ordinary checks name what is wrong with it
			}
			return problem("%s: this chat is about %s, so it cannot change %s", path, c.describe(), sectionName(target))
		}
		if !c.isList() {
			continue
		}
		switch ch.Op {
		case opAdd:
			if len(c.ItemIDs) > 0 {
				return problem("%s: this chat is about %s, so it cannot add a row", path, c.describe())
			}
		case opUpdate, opRemove:
			if id := strings.TrimSpace(ch.ID); id != "" && !c.holdsRow(id) {
				return problem("%s: this chat is about %s, and the row %q is not one of them", path, c.describe(), id)
			}
		}
	}
	return nil
}

// ChangeSuggestionsIn is ChangeSuggestions for a call that belongs to a conversation: a conversation that is bound to a section may only change that run's section or rows.
func ChangeSuggestionsIn(conversationID, runID string, changes []SuggestionChange, basedOn string) (SuggestionsChanged, error) {
	chat, err := sectionChatOf(conversationID)
	if err != nil {
		return SuggestionsChanged{}, err
	}
	if chat != nil {
		if runID != chat.RunID {
			return SuggestionsChanged{}, problem("this chat is about another import (%s): it cannot change import %s", chat.RunID, runID)
		}
		if err := chat.checkChangesInScope(changes); err != nil {
			return SuggestionsChanged{}, err
		}
	}
	return ChangeSuggestions(runID, changes, basedOn)
}

// GetSuggestionsIn is what get_cv_import_suggestions answers for a call that belongs to a conversation. With no section and no binding it is the whole of GetSuggestions, as before. A
// section (and ids) cuts the answer to that part; a bound conversation is always cut to what it is bound to and cannot ask for more. The revision is always that of the whole proposal.
func GetSuggestionsIn(conversationID, runID, section string, ids []string) (any, error) {
	chat, err := sectionChatOf(conversationID)
	if err != nil {
		return nil, err
	}
	section = strings.TrimSpace(section)
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			cleaned = append(cleaned, id)
		}
	}
	scopeText := ""
	if chat != nil {
		if runID != chat.RunID {
			return nil, problem("this chat is about another import (%s): it cannot read import %s", chat.RunID, runID)
		}
		if section != "" && section != chat.Section {
			return nil, problem("this chat is about %s, so it cannot read the %s section", chat.describe(), sectionName(section))
		}
		for _, id := range cleaned {
			if !chat.holdsRow(id) {
				return nil, problem("this chat is about %s, and the row %q is not one of them", chat.describe(), id)
			}
		}
		section = chat.Section
		if len(cleaned) == 0 {
			cleaned = chat.ItemIDs
		}
		scopeText = "This chat is about " + chat.describe() + "."
	}
	s, err := GetSuggestions(runID)
	if err != nil {
		return nil, err
	}
	if section == "" {
		if len(cleaned) > 0 {
			return nil, problem("ids go with a section: name skills, experience or education")
		}
		return s, nil
	}
	if !contains(sections, section) && !contains(lists, section) {
		return nil, problem("section must be one of %s, %s", strings.Join(sections, ", "), strings.Join(lists, ", "))
	}
	if len(cleaned) > 0 && !contains(lists, section) {
		return nil, problem("%s is a single section: ids only go with skills, experience or education", section)
	}
	out := map[string]any{"runId": s.RunID, "status": s.Status, "originalFilename": s.OriginalFilename, "revision": s.Revision, "section": section}
	if scopeText != "" {
		out["scope"] = scopeText
	}
	switch section {
	case "profile":
		out["profile"] = s.Profile
	case "persona":
		out["persona"] = s.Persona
	case "personalDetails":
		out["personalDetails"] = s.PersonalDetails
	case "skills":
		rows := []skillSuggestion{}
		for _, r := range s.Skills {
			if len(cleaned) == 0 || contains(cleaned, r.Id) {
				rows = append(rows, r)
			}
		}
		if err := wantAll(cleaned, len(rows), section); err != nil {
			return nil, err
		}
		out["skills"] = rows
	case "experience":
		rows := []experienceSuggestion{}
		for _, r := range s.Experience {
			if len(cleaned) == 0 || contains(cleaned, r.Id) {
				rows = append(rows, r)
			}
		}
		if err := wantAll(cleaned, len(rows), section); err != nil {
			return nil, err
		}
		out["experience"] = rows
	default:
		rows := []educationSuggestion{}
		for _, r := range s.Education {
			if len(cleaned) == 0 || contains(cleaned, r.Id) {
				rows = append(rows, r)
			}
		}
		if err := wantAll(cleaned, len(rows), section); err != nil {
			return nil, err
		}
		out["education"] = rows
	}
	return out, nil
}

// wantAll checks that every distinct id that was asked for was found among the rows.
func wantAll(asked []string, found int, section string) error {
	distinct := map[string]bool{}
	for _, id := range asked {
		distinct[id] = true
	}
	if len(distinct) > 0 && found != len(distinct) {
		return problem("some of the ids are not rows of %s: read them again without ids to see the current rows", section)
	}
	return nil
}

type sectionChatRequest struct {
	RunID          string   `json:"runId"`
	ConversationID string   `json:"conversationId"`
	Section        string   `json:"section"`
	ItemIDs        []string `json:"itemIds"`
}

// SectionChatsHandler handles POST cv-import/section-chats: the review page records which section or rows a new chat is for, before its first message.
func SectionChatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body sectionChatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.RunID) == "" || strings.TrimSpace(body.ConversationID) == "" || strings.TrimSpace(body.Section) == "" {
		http.Error(w, "runId, conversationId and section are required (itemIds is optional)", http.StatusBadRequest)
		return
	}
	id, err := RecordSectionChat(body.RunID, body.ConversationID, body.Section, body.ItemIDs)
	var bad *proposalProblem
	switch {
	case errors.Is(err, errUnknownCVImportRun), errors.Is(err, sql.ErrNoRows):
		http.Error(w, "unknown cv import run id", http.StatusNotFound)
	case errors.As(err, &bad):
		http.Error(w, bad.msg, http.StatusBadRequest)
	case errors.Is(err, errChatAlreadyBound), errors.Is(err, errChatRunClosed), errors.Is(err, errSuggestionsStillGenerating), errors.Is(err, errNoSuggestions):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		// a run that failed or has no proposal is as closed to a chat as a saved one; any other failure is the server's
		if strings.HasPrefix(err.Error(), "this CV import failed") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "failed to record the chat: "+err.Error(), http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	}
}
