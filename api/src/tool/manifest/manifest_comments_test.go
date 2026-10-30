package manifest

import (
	"strings"
	"testing"
)

func commentsManifest(c ...CommentTypeDecl) Manifest {
	return Manifest{
		Slug: "demo", Name: "Demo", Version: "1.0.0",
		Scopes:   []ScopeDecl{{Scope: "tool:demo:read"}, {Scope: "tool:demo:write"}},
		Comments: c,
	}
}

func okComment() CommentTypeDecl {
	return CommentTypeDecl{EntityType: "job", ReadScope: "tool:demo:read", WriteScope: "tool:demo:write"}
}

func TestValidateComments(t *testing.T) {
	// A frontend-only tool needs no backend for comments: core stores them.
	frontendOnly := ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0"}

	if _, err := Validate(commentsManifest(okComment()), frontendOnly); err != nil {
		t.Fatalf("a valid comments declaration was rejected: %v", err)
	}
	two := okComment()
	two.EntityType = "note"
	same := okComment()
	same.WriteScope = same.ReadScope // one scope for both is fine
	if _, err := Validate(commentsManifest(okComment(), two, CommentTypeDecl{EntityType: "company", ReadScope: same.ReadScope, WriteScope: same.WriteScope}), frontendOnly); err != nil {
		t.Fatalf("several types rejected: %v", err)
	}
	if _, err := Validate(commentsManifest(), frontendOnly); err != nil {
		t.Fatalf("no comments rejected: %v", err)
	}

	for name, mutate := range map[string]func(*CommentTypeDecl){
		"empty type":         func(c *CommentTypeDecl) { c.EntityType = "" },
		"upper-case type":    func(c *CommentTypeDecl) { c.EntityType = "Job" },
		"path-like type":     func(c *CommentTypeDecl) { c.EntityType = "../job" },
		"type with a space":  func(c *CommentTypeDecl) { c.EntityType = "my job" },
		"type too long":      func(c *CommentTypeDecl) { c.EntityType = "a" + strings.Repeat("b", 40) },
		"no read scope":      func(c *CommentTypeDecl) { c.ReadScope = "" },
		"no write scope":     func(c *CommentTypeDecl) { c.WriteScope = "" },
		"foreign read scope": func(c *CommentTypeDecl) { c.ReadScope = "tool:other:read" },
		"foreign write":      func(c *CommentTypeDecl) { c.WriteScope = "cinqo:super:admin" },
		"undeclared scope":   func(c *CommentTypeDecl) { c.WriteScope = "tool:demo:nope" },
	} {
		c := okComment()
		mutate(&c)
		if _, err := Validate(commentsManifest(c), frontendOnly); err == nil {
			t.Errorf("%s: accepted", name)
		} else if !strings.Contains(err.Error(), "comments") {
			t.Errorf("%s: the message should say it is about comments: %v", name, err)
		}
	}

	if _, err := Validate(commentsManifest(okComment(), okComment()), frontendOnly); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate type: %v", err)
	}
}
