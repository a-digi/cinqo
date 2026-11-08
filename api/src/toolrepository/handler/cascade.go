package handler

import (
	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/toolrepository/resolver"
)

// planAlternative is another tool to update instead, which makes room for the one that was asked for (plan dependency-install, step 10): "Career works
// only with Browser up to 1.5, so Browser cannot be updated; a newer Career works with the new Browser, update Career, and Browser comes with it".
// Only offered; the plan never updates a dependent by itself.
type planAlternative struct {
	ForToolID      string `json:"forToolId"`
	ToolID         string `json:"toolId"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	VersionID      string `json:"versionId"`
	RepositoryID   string `json:"repositoryId"`
	RepositoryName string `json:"repositoryName"`
}

// cascadeAlternatives finds, for each tool a plan could not update because an installed tool pins it (would_break_installed), the installed tools that
// need it and have a newer version whose own plan is possible and updates the blocked tool too. Read only, from the stored repository files.
func cascadeAlternatives(reqCtx request.RequestContext, plan *resolver.Plan, installed []resolver.InstalledTool, env resolver.Env, chosenRepo string) []planAlternative {
	out := []planAlternative{}
	seen := map[string]bool{}
	for _, pr := range plan.Problems {
		if pr.Kind != resolver.ProblemWouldBreakInstalled {
			continue
		}
		for _, d := range installed {
			if seen[pr.ToolID+">"+d.ID] || !needs(d, pr.ToolID) {
				continue
			}
			repos := []string{d.SourceRepositoryID, chosenRepo}
			for _, rid := range repos {
				if rid == "" {
					continue
				}
				alt, err := svc(reqCtx).PlanDependencies(rid, d.ID, "", installed, env)
				if err != nil || !alt.OK {
					continue
				}
				var self *resolver.Step
				covers := false
				for i := range alt.Steps {
					switch {
					case alt.Steps[i].ToolID == d.ID && alt.Steps[i].Action == resolver.ActionUpdate:
						self = &alt.Steps[i]
					case alt.Steps[i].ToolID == pr.ToolID && alt.Steps[i].Action == resolver.ActionUpdate:
						covers = true
					}
				}
				if self == nil || !covers {
					continue
				}
				seen[pr.ToolID+">"+d.ID] = true
				out = append(out, planAlternative{ForToolID: pr.ToolID, ToolID: d.ID, Name: d.Name, Version: self.Version, VersionID: self.VersionID, RepositoryID: self.RepositoryID, RepositoryName: self.RepositoryName})
				break
			}
		}
	}
	return out
}

func needs(t resolver.InstalledTool, id string) bool {
	for _, r := range t.Requires {
		if r.ID == id {
			return true
		}
	}
	return false
}
