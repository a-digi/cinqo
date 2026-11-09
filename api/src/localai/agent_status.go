package localai

import (
	"math"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/system"
)

// agentStatus turns the program's state into what the resource bar shows. A program that does not run is "none" (the bar says no local
// agent runs); a start that is under way is "starting"; a loaded program with a request under way is "working", without one "idle"; one
// that died on its own is "failed" until the next start.
func agentStatus(v agentView) system.AgentStatus {
	st := system.AgentStatus{Name: modelName(v.modelID)}
	switch v.state {
	case entity.EngineStarting:
		st.State = system.AgentStarting
	case entity.EngineReady:
		st.State = system.AgentIdle
		if v.busy {
			st.State = system.AgentWorking
		}
		st.UnloadsInSeconds = int(math.Ceil(v.unloadsIn.Seconds()))
	case entity.EngineFailed:
		st.State = system.AgentFailed
	default:
		st.State = system.AgentNone
	}
	return st
}

// modelName is the name a person knows the model by; the id when the model is not in the list any more, and empty without an id.
func modelName(id string) string {
	if id == "" {
		return ""
	}
	if c, err := models.Current(); err == nil {
		for _, m := range c.Models {
			if m.ID == id {
				return m.Name
			}
		}
	}
	return id
}
