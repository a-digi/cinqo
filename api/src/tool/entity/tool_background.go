package entity

// ToolBackgroundRequest is one route of another tool that an installed tool asked to use without a signed-in person (its manifest background_access). It
// grants nothing: only a ToolBackgroundGrant, made by a person, does.
type ToolBackgroundRequest struct {
	ToolID     string `json:"tool_id"`
	TargetSlug string `json:"target_slug"`
	Method     string `json:"method"`
	PathSuffix string `json:"path_suffix"`
}

// ToolBackgroundGrant is a request an administrator approved. A tool is named by its slug on the target side, like requires_tools does.
type ToolBackgroundGrant struct {
	ToolBackgroundRequest
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
}
