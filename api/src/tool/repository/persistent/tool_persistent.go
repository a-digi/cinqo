package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolPersistentRepo struct {
	db *sql.DB
}

func NewToolPersistentRepo(db *sql.DB) *ToolPersistentRepo {
	return &ToolPersistentRepo{db: db}
}

// InsertFresh inserts a brand-new tool row plus its declared child rows
// (scopes/routes/required-scopes), in one transaction — see
// plan/ai/tools/step-03-install-update-uninstall.md.
func (r *ToolPersistentRepo) InsertFresh(
	t *tool_entity.Tool,
	scopes []tool_entity.ToolScope,
	routes []tool_entity.ToolRoute,
	required []tool_entity.ToolRequiredScope,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path,
			backend_executable_relpath, frontend_bundle_relpath, min_app_version, max_app_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		t.ID, t.Slug, t.Name, t.Version, t.Kind, t.Enabled, t.Status, t.InstallPath,
		t.BackendExecutableRelpath, t.FrontendBundleRelpath, t.MinAppVersion, t.MaxAppVersion,
	); err != nil {
		return err
	}

	if err := insertChildRows(tx, t.ID, scopes, routes, required); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateVersion replaces an existing tool's version/kind/paths and
// wholesale-replaces its declared child rows with the new manifest's
// own declarations (deleted then re-inserted, in the same
// transaction as the row update). enabled/status are taken from t as
// the caller decided (an update preserves the tool's prior enabled
// state — see the step-03 design doc).
func (r *ToolPersistentRepo) UpdateVersion(
	t *tool_entity.Tool,
	scopes []tool_entity.ToolScope,
	routes []tool_entity.ToolRoute,
	required []tool_entity.ToolRequiredScope,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE tools SET name = ?, version = ?, kind = ?, enabled = ?, status = ?, install_path = ?,
			backend_executable_relpath = ?, frontend_bundle_relpath = ?, min_app_version = ?, max_app_version = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		t.Name, t.Version, t.Kind, t.Enabled, t.Status, t.InstallPath,
		t.BackendExecutableRelpath, t.FrontendBundleRelpath, t.MinAppVersion, t.MaxAppVersion,
		t.ID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM tool_scopes WHERE tool_id = ?`, t.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tool_routes WHERE tool_id = ?`, t.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tool_required_scopes WHERE tool_id = ?`, t.ID); err != nil {
		return err
	}

	if err := insertChildRows(tx, t.ID, scopes, routes, required); err != nil {
		return err
	}

	return tx.Commit()
}

// SetEnabled flips a tool's enabled flag. The manager package (Start/Stop)
// is responsible for actually starting/stopping the tool's backend
// process around this call — see
// plan/ai/tools/step-04-enable-disable-and-tool-manager.md.
func (r *ToolPersistentRepo) SetEnabled(id string, enabled bool) error {
	_, err := r.db.Exec(`UPDATE tools SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabled, id)
	return err
}

// Delete removes a tool's row and every child row, in one transaction.
// PRAGMA foreign_keys is never enabled in this codebase (verified
// directly against coco-mda's own equivalent, and grepped cinqo's own
// source for confirmation — see step 1) — cascade never fires, so
// every child table is deleted explicitly here.
func (r *ToolPersistentRepo) Delete(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_routes WHERE tool_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tool_scopes WHERE tool_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tool_required_scopes WHERE tool_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tools WHERE id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

func insertChildRows(
	tx *sql.Tx,
	toolID string,
	scopes []tool_entity.ToolScope,
	routes []tool_entity.ToolRoute,
	required []tool_entity.ToolRequiredScope,
) error {
	for _, s := range scopes {
		if _, err := tx.Exec(
			`INSERT INTO tool_scopes (tool_id, scope, description) VALUES (?, ?, ?)`,
			toolID, s.Scope, s.Description,
		); err != nil {
			return err
		}
	}
	for _, rt := range routes {
		if _, err := tx.Exec(
			`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope) VALUES (?, ?, ?, ?)`,
			toolID, rt.Method, rt.PathSuffix, rt.RequiredScope,
		); err != nil {
			return err
		}
	}
	for _, s := range required {
		if _, err := tx.Exec(
			`INSERT INTO tool_required_scopes (tool_id, scope) VALUES (?, ?)`,
			toolID, s.Scope,
		); err != nil {
			return err
		}
	}
	return nil
}
