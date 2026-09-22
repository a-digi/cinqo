// Package entity holds the tool (plugin) system's persisted shapes —
// see plan/ai/tools/step-01-data-model-and-migration.md.
package entity

// Tool is one installed tool. Kind is derived at install time from
// which of frontend/backend the uploaded package actually contains —
// never supplied directly by the tool's own manifest. InstallPath and
// PID are server-internal filesystem/OS detail, never handed to a
// frontend caller.
type Tool struct {
	_                        struct{} `table:"tools"`
	ID                       string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	Slug                     string   `db:"slug" dbtype:"TEXT" nullable:"false" json:"slug"`
	Name                     string   `db:"name" dbtype:"TEXT" nullable:"false" json:"name"`
	Version                  string   `db:"version" dbtype:"TEXT" nullable:"false" json:"version"`
	Kind                     string   `db:"kind" dbtype:"TEXT" nullable:"false" json:"kind"`
	Enabled                  bool     `db:"enabled" dbtype:"INTEGER" nullable:"false" json:"enabled"`
	Status                   string   `db:"status" dbtype:"TEXT" nullable:"false" json:"status"`
	InstallPath              string   `db:"install_path" dbtype:"TEXT" nullable:"false" json:"-"`
	BackendExecutableRelpath string   `db:"backend_executable_relpath" dbtype:"TEXT" nullable:"true" json:"-"`
	FrontendBundleRelpath    string   `db:"frontend_bundle_relpath" dbtype:"TEXT" nullable:"true" json:"frontend_bundle_relpath"`
	MinAppVersion            string   `db:"min_app_version" dbtype:"TEXT" nullable:"true" json:"min_app_version"`
	MaxAppVersion            string   `db:"max_app_version" dbtype:"TEXT" nullable:"true" json:"max_app_version"`
	PID                      int      `db:"pid" dbtype:"INTEGER" nullable:"true" json:"-"`
	CreatedAt                string   `db:"created_at" dbtype:"DATETIME" nullable:"false" json:"created_at"`
	UpdatedAt                string   `db:"updated_at" dbtype:"DATETIME" nullable:"true" json:"updated_at"`
	// ServiceToken is this tool's own persistent, machine-identity
	// bearer credential — never a per-user permission, never exposed to
	// a frontend caller (json:"-", like InstallPath/PID above). Lets
	// this tool's own backend process authenticate a call back into
	// core (currently: POST /api/v1/events/publish) without borrowing
	// any end user's own session token. Generated once at first
	// successful install, passed to the tool's own subprocess as
	// TOOL_SERVICE_TOKEN (tool/manager.go's own ToolEnvVars), preserved
	// (never silently regenerated) across a version update. See
	// plan/ai/domain-events/step-05-tool-publish-endpoint.md.
	ServiceToken string `db:"service_token" dbtype:"TEXT" nullable:"false" json:"-"`
}
