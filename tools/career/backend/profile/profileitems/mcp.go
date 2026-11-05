package profileitems

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// The AI tools of the profile library (plan 173, step 2). The older tools add_career_skill, add_career_experience and the like keep
// working on a persona's OWN custom items; these work on the profile's library and on what a persona uses of it.

func errResult(action string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, ErrUnknownProfile):
		return db.ErrResult("unknown profile id")
	case errors.Is(err, ErrUnknownPersona):
		return db.ErrResult("unknown persona id")
	case errors.Is(err, ErrUnknownItem):
		return db.ErrResult("unknown skill or experience id (see list_profile_skills and list_profile_experience)")
	case errors.Is(err, ErrOtherProfile), errors.Is(err, ErrSkillRequired), errors.Is(err, ErrExperienceRequired), errors.Is(err, ErrTitleOverrideEmpty):
		return db.ErrResult(err.Error())
	}
	return db.ErrResult(fmt.Sprintf("failed to %s: %v", action, err))
}

type profileArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile (from list_profiles)"`
}

func RegisterListProfileSkills(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_profile_skills",
		Description: "List the skills in a profile's library with their ids and how many personas use each. The library is global to the profile; each persona uses some of it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args profileArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		list, err := ListSkills(args.ProfileID)
		if err != nil {
			return errResult("list skills", err), nil, nil
		}
		return db.JSONResult(map[string]any{"skills": list})
	})
}

type addProfileSkillArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile to add the skill to"`
	Skill     string `json:"skill" jsonschema:"the skill, e.g. \"Go\""`
}

func RegisterAddProfileSkill(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_profile_skill",
		Description: "Add a skill to a profile's library (global to the profile, not yet used by any persona; choose it for a persona with set_persona_skills). A skill the profile already has, in any case, is returned instead of added again. Returns the skill's id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addProfileSkillArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		skill, created, err := AddSkill(args.ProfileID, args.Skill)
		if err != nil {
			return errResult("add skill", err), nil, nil
		}
		return db.JSONResult(map[string]any{"id": skill.ID, "skill": skill.Skill, "created": created})
	})
}

func RegisterListProfileExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_profile_experience",
		Description: "List the work experience in a profile's library with ids and how many personas use each. The library is global to the profile; each persona uses some of it and may override the title and description of an entry for its role.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args profileArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		list, err := ListExperience(args.ProfileID)
		if err != nil {
			return errResult("list experience", err), nil, nil
		}
		return db.JSONResult(map[string]any{"experience": list})
	})
}

type addProfileExperienceArgs struct {
	ProfileID   string `json:"profileId" jsonschema:"the profile to add the experience to"`
	Company     string `json:"company" jsonschema:"the employer's name"`
	Title       string `json:"title" jsonschema:"the job title held there"`
	StartDate   string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     string `json:"endDate,omitempty" jsonschema:"when this position ended — omit for a current position"`
	Description string `json:"description,omitempty" jsonschema:"what the role involved"`
}

func RegisterAddProfileExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_profile_experience",
		Description: "Add a work experience entry to a profile's library (global to the profile, not yet used by any persona; choose it for a persona with set_persona_experience). Returns the entry's id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addProfileExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		e, err := AddExperience(args.ProfileID, ExperienceInput{
			Company: args.Company, Title: args.Title, StartDate: args.StartDate, EndDate: args.EndDate, Description: args.Description,
		})
		if err != nil {
			return errResult("add experience", err), nil, nil
		}
		return db.JSONResult(map[string]string{"id": e.ID})
	})
}

type setPersonaSkillsArgs struct {
	PersonaID string   `json:"personaId" jsonschema:"the persona"`
	SkillIDs  []string `json:"skillIds" jsonschema:"the ids (from list_profile_skills) of exactly the library skills this persona uses; an empty list means none. Every id must be a skill of the persona's own profile."`
}

func RegisterSetPersonaSkills(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_persona_skills",
		Description: "Choose which of the profile's library skills a persona uses: replaces the persona's whole choice with exactly these ids. The persona's own custom skills (add_career_skill) are not affected. Nothing changes if any id is refused.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setPersonaSkillsArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if err := SetPersonaSkills(args.PersonaID, args.SkillIDs); err != nil {
			return errResult("set the persona's skills", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type setPersonaExperienceArgs struct {
	PersonaID  string          `json:"personaId" jsonschema:"the persona"`
	Experience []ExperienceUse `json:"experience" jsonschema:"exactly the library experiences this persona uses, each with its id (from list_profile_experience) and optionally titleOverride and descriptionOverride (the persona's own wording for its role; leave them out to follow the profile's text; a title override cannot be empty, a description override may be empty). An empty list means none."`
}

func RegisterSetPersonaExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_persona_experience",
		Description: "Choose which of the profile's library experiences a persona uses, and how it words them for its role: replaces the persona's whole choice (and every override) with exactly this list. The persona's own custom entries (add_career_experience) are not affected. Nothing changes if any id is refused.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setPersonaExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if err := SetPersonaExperience(args.PersonaID, args.Experience); err != nil {
			return errResult("set the persona's experience", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}
