package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/tool/manifest"
)

func TestCacheCommentTypes(t *testing.T) {
	di, _ := newUploadDI(t)
	if _, err := di.db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES ('t1', 'demo', 'Demo', '1.0.0', 'frontend_only', 1, 'running', '/x', '2026-10-30T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("POST", "/x", nil), di)

	read := func() map[string][2]string {
		rows, err := di.db.Query(`SELECT entity_type, read_scope, write_scope FROM tool_comment_types WHERE tool_id = 't1'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string][2]string{}
		for rows.Next() {
			var typ, r, w string
			if err := rows.Scan(&typ, &r, &w); err != nil {
				t.Fatal(err)
			}
			out[typ] = [2]string{r, w}
		}
		return out
	}

	m := manifest.Manifest{Slug: "demo", Comments: []manifest.CommentTypeDecl{
		{EntityType: "job", ReadScope: "tool:demo:read", WriteScope: "tool:demo:write"},
		{EntityType: "note", ReadScope: "tool:demo:read", WriteScope: "tool:demo:read"},
	}}
	cacheCommentTypes(ctx, di.db, m, "t1")
	got := read()
	if len(got) != 2 || got["job"] != [2]string{"tool:demo:read", "tool:demo:write"} || got["note"] != [2]string{"tool:demo:read", "tool:demo:read"} {
		t.Fatalf("cached = %v", got)
	}

	// An update that declares one type replaces the set, and one declaring none clears it.
	cacheCommentTypes(ctx, di.db, manifest.Manifest{Slug: "demo", Comments: m.Comments[:1]}, "t1")
	if got := read(); len(got) != 1 || got["job"][0] == "" {
		t.Errorf("after an update with one type: %v", got)
	}
	cacheCommentTypes(ctx, di.db, manifest.Manifest{Slug: "demo"}, "t1")
	if got := read(); len(got) != 0 {
		t.Errorf("after an update without comments: %v", got)
	}
}
