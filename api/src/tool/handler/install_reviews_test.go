package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/tool/manifest"
)

func TestCacheReviewTypes(t *testing.T) {
	di, _ := newUploadDI(t)
	if _, err := di.db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES ('t1', 'demo', 'Demo', '1.0.0', 'frontend_only', 1, 'running', '/x', '2026-11-09T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("POST", "/x", nil), di)

	type row struct {
		scale int
		half  int
		r, w  string
	}
	read := func() map[string]row {
		rows, err := di.db.Query(`SELECT entity_type, scale, half_stars, read_scope, write_scope FROM tool_review_types WHERE tool_id = 't1'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var typ string
			var x row
			if err := rows.Scan(&typ, &x.scale, &x.half, &x.r, &x.w); err != nil {
				t.Fatal(err)
			}
			out[typ] = x
		}
		return out
	}

	m := manifest.Manifest{Slug: "demo", Reviews: []manifest.ReviewTypeDecl{
		{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "tool:demo:read", WriteScope: "tool:demo:write"},
		{EntityType: "job", Scale: 10, ReadScope: "tool:demo:read", WriteScope: "tool:demo:read"},
	}}
	cacheReviewTypes(ctx, di.db, m, "t1")
	got := read()
	if len(got) != 2 || got["company"] != (row{5, 1, "tool:demo:read", "tool:demo:write"}) || got["job"] != (row{10, 0, "tool:demo:read", "tool:demo:read"}) {
		t.Fatalf("cached = %v", got)
	}

	// An update that declares one type replaces the set, and one declaring none clears it.
	cacheReviewTypes(ctx, di.db, manifest.Manifest{Slug: "demo", Reviews: m.Reviews[:1]}, "t1")
	if got := read(); len(got) != 1 || got["company"].scale != 5 {
		t.Errorf("after an update with one type: %v", got)
	}
	cacheReviewTypes(ctx, di.db, manifest.Manifest{Slug: "demo"}, "t1")
	if got := read(); len(got) != 0 {
		t.Errorf("after an update without reviews: %v", got)
	}
}
