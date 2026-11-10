package localai

import (
	"sync"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/platform"
)

// RunningTurn is a conversation of the local platform that has a turn under way: what the application tells the module through Deps, because the
// module may not read the conversations itself.
type RunningTurn struct {
	ConversationID string
	Title          string
	UserID         string
	Model          string
}

var (
	runningMu    sync.RWMutex
	runningTurns func() []RunningTurn
)

// setRunningTurns remembers where to ask for the running turns (nil: nobody is known to use the model).
func setRunningTurns(f func() []RunningTurn) {
	runningMu.Lock()
	runningTurns = f
	runningMu.Unlock()
}

func currentRunningTurns() []RunningTurn {
	runningMu.RLock()
	f := runningTurns
	runningMu.RUnlock()
	if f == nil {
		return nil
	}
	return f()
}

// occupantFor says what a new conversation on target would displace (plan 241): the model program that is loaded or loading and is another model
// than target. Nothing loaded, a program that failed, or the same model is no conflict (nil). A model with a turn under way is in use; a request
// with no turn (a test of the model) also counts as in use, without a conversation to name; a start that is under way is loading; the rest is idle.
func occupantFor(v agentView, target string, running []RunningTurn, name func(string) string) *platform.Occupant {
	if v.modelID == "" || v.modelID == target {
		return nil
	}
	if v.state != entity.EngineStarting && v.state != entity.EngineReady {
		return nil
	}
	o := &platform.Occupant{ModelID: v.modelID, ModelName: name(v.modelID)}
	for _, t := range running {
		if t.Model == v.modelID {
			o.Conversations = append(o.Conversations, platform.OccupantConversation{ID: t.ConversationID, Title: t.Title, UserID: t.UserID})
		}
	}
	switch {
	case len(o.Conversations) > 0 || (v.state == entity.EngineReady && v.busy):
		o.State = platform.OccupantInUse
	case v.state == entity.EngineStarting:
		o.State = platform.OccupantLoading
	default:
		o.State = platform.OccupantIdle
	}
	return o
}

// occupantOfEngine is the platform's Occupant seam: what the managed engine has loaded, read now.
func occupantOfEngine(model string) *platform.Occupant {
	if managed == nil {
		return nil
	}
	return occupantFor(managed.sup.view(), model, currentRunningTurns(), modelName)
}

// releaseEngine is the platform's Release seam: unloads the model program without forcing it (plan 225); false while it is in use.
func releaseEngine(string) bool {
	if managed == nil {
		return true
	}
	return managed.Unload(false) == entity.UnloadStopped
}
