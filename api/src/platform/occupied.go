package platform

import (
	"fmt"
	"strings"
)

// The states of an Occupant: what the platform has loaded and whether anything is using it.
const (
	// OccupantLoading: the model is being loaded and nobody is using it yet.
	OccupantLoading = "loading"
	// OccupantIdle: the model is loaded and nothing is using it.
	OccupantIdle = "idle"
	// OccupantInUse: a conversation, or another request, is using the model right now.
	OccupantInUse = "in_use"
)

// ConflictCode is the code of the answer a start gets when it would displace another model.
const ConflictCode = "local_model_occupied"

// OccupantConversation is a conversation that is using the loaded model: its id, its title and its owner.
type OccupantConversation struct {
	ID     string
	Title  string
	UserID string
}

// Occupant is what a platform has loaded that a new conversation on another model would displace.
type Occupant struct {
	ModelID   string
	ModelName string
	// State is OccupantLoading, OccupantIdle or OccupantInUse.
	State string
	// Conversations are the conversations that have a turn under way on the model (empty while it is only loaded, and possibly empty while it is in
	// use: a request that belongs to no conversation, such as a test of the model).
	Conversations []OccupantConversation
}

// ConversationRef is a conversation as the answer names it: the id and the title only.
type ConversationRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Conflict is the answer to a start that would displace another model: what the caller shows and what it may do about it.
type Conflict struct {
	Code            string
	Message         string
	LoadedModel     string
	LoadedModelName string
	State           string
	// Conversations are only the caller's own conversations; another person's conversation is counted in OthersUsing, never named.
	Conversations []ConversationRef
	OthersUsing   int
	// CanRelease is true when the caller may unload the other model and it is not in use.
	CanRelease bool
}

// CheckOccupied says whether starting a conversation on model would displace another model of this platform, and if so, what to tell the
// person. callerID is the person who starts it (only their own conversations are named); mayRelease says whether they may unload a model. Nil when
// there is no conflict, which is always the case for a platform without the Occupant seam.
func (e Entry) CheckOccupied(model, callerID string, mayRelease bool) *Conflict {
	if e.Occupant == nil {
		return nil
	}
	o := e.Occupant(model)
	if o == nil {
		return nil
	}
	name := o.ModelName
	if name == "" {
		name = o.ModelID
	}
	target := model
	if e.ModelNames != nil {
		if n := e.ModelNames()[model]; n != "" {
			target = n
		}
	}
	c := &Conflict{Code: ConflictCode, LoadedModel: o.ModelID, LoadedModelName: name, State: o.State}
	for _, conv := range o.Conversations {
		if conv.UserID == callerID {
			c.Conversations = append(c.Conversations, ConversationRef{ID: conv.ID, Title: conv.Title})
		} else {
			c.OthersUsing++
		}
	}
	c.CanRelease = mayRelease && o.State != OccupantInUse && e.Release != nil
	c.Message = conflictMessage(name, target, o.State, c.Conversations, c.OthersUsing)
	return c
}

// ReleaseOccupant unloads what Occupant named, without forcing; false when the platform cannot, or the model is in use at that moment.
func (e Entry) ReleaseOccupant(model string) bool {
	if e.Release == nil {
		return false
	}
	return e.Release(model)
}

func conflictMessage(loaded, target, state string, own []ConversationRef, others int) string {
	if state != OccupantInUse {
		return fmt.Sprintf("%s is loaded. Unload it before starting a conversation on %s.", loaded, target)
	}
	var who string
	switch {
	case len(own) > 0 && others > 0:
		who = fmt.Sprintf("in %s and %s", quoteTitles(own), countOthers(others))
	case len(own) > 0:
		who = "in " + quoteTitles(own)
	case others > 0:
		who = "in " + countOthers(others)
	default:
		return fmt.Sprintf("%s is busy answering a request. Wait until it is done, then start this conversation on %s.", loaded, target)
	}
	return fmt.Sprintf("%s is answering %s. Stop that conversation first, then start this one on %s.", loaded, who, target)
}

func quoteTitles(refs []ConversationRef) string {
	parts := make([]string, len(refs))
	for i, r := range refs {
		parts[i] = fmt.Sprintf("the conversation %q", r.Title)
	}
	return strings.Join(parts, " and ")
}

func countOthers(n int) string {
	if n == 1 {
		return "another conversation"
	}
	return fmt.Sprintf("%d other conversations", n)
}
