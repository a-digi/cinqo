package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func reviewsManifest(r ...ReviewTypeDecl) Manifest {
	return Manifest{
		Info: testInfo(),
		Slug: "demo", Name: "Demo", Version: "1.0.0",
		Scopes:  []ScopeDecl{{Scope: "tool:demo:read"}, {Scope: "tool:demo:write"}},
		Reviews: r,
	}
}

func okReview() ReviewTypeDecl {
	return ReviewTypeDecl{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "tool:demo:read", WriteScope: "tool:demo:write"}
}

func TestValidateReviews(t *testing.T) {
	// A frontend-only tool needs no backend for reviews: core stores them.
	frontendOnly := ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0"}

	if _, err := Validate(reviewsManifest(okReview()), frontendOnly); err != nil {
		t.Fatalf("a valid reviews declaration was rejected: %v", err)
	}
	for _, scale := range []int{3, 5, 10} {
		r := okReview()
		r.Scale = scale
		r.HalfStars = scale != 5
		if _, err := Validate(reviewsManifest(r), frontendOnly); err != nil {
			t.Errorf("scale %d rejected: %v", scale, err)
		}
	}
	job := okReview()
	job.EntityType = "job"
	same := okReview()
	same.EntityType = "persona"
	same.WriteScope = same.ReadScope // one scope for both is fine
	if _, err := Validate(reviewsManifest(okReview(), job, same), frontendOnly); err != nil {
		t.Fatalf("several types rejected: %v", err)
	}
	if _, err := Validate(reviewsManifest(), frontendOnly); err != nil {
		t.Fatalf("no reviews rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ReviewTypeDecl){
		"empty type":         func(r *ReviewTypeDecl) { r.EntityType = "" },
		"upper-case type":    func(r *ReviewTypeDecl) { r.EntityType = "Company" },
		"path-like type":     func(r *ReviewTypeDecl) { r.EntityType = "../company" },
		"type with a space":  func(r *ReviewTypeDecl) { r.EntityType = "my company" },
		"type too long":      func(r *ReviewTypeDecl) { r.EntityType = "a" + strings.Repeat("b", 40) },
		"no scale":           func(r *ReviewTypeDecl) { r.Scale = 0 },
		"scale 4":            func(r *ReviewTypeDecl) { r.Scale = 4 },
		"scale 11":           func(r *ReviewTypeDecl) { r.Scale = 11 },
		"negative scale":     func(r *ReviewTypeDecl) { r.Scale = -5 },
		"no read scope":      func(r *ReviewTypeDecl) { r.ReadScope = "" },
		"no write scope":     func(r *ReviewTypeDecl) { r.WriteScope = "" },
		"foreign read scope": func(r *ReviewTypeDecl) { r.ReadScope = "tool:other:read" },
		"foreign write":      func(r *ReviewTypeDecl) { r.WriteScope = "cinqo:super:admin" },
		"undeclared scope":   func(r *ReviewTypeDecl) { r.WriteScope = "tool:demo:nope" },
	} {
		r := okReview()
		mutate(&r)
		if _, err := Validate(reviewsManifest(r), frontendOnly); err == nil {
			t.Errorf("%s: accepted", name)
		} else if !strings.Contains(err.Error(), "reviews") {
			t.Errorf("%s: the message should say it is about reviews: %v", name, err)
		}
	}
	if _, err := Validate(reviewsManifest(okReview(), okReview()), frontendOnly); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate type: %v", err)
	}
	bad := okReview()
	bad.Scale = 4
	if _, err := Validate(reviewsManifest(bad), frontendOnly); err == nil || !strings.Contains(err.Error(), "3, 5 or 10") {
		t.Errorf("the scale message: %v", err)
	}
}

func TestReviewsAreReadFromTheManifestJSON(t *testing.T) {
	raw := `{"reviews":[{"entity_type":"company","scale":5,"half_stars":true,"read_scope":"a","write_scope":"b"},{"entity_type":"job","scale":10,"read_scope":"a","write_scope":"b"}]}`
	var m Manifest
	if err := jsonUnmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Reviews) != 2 || m.Reviews[0] != (ReviewTypeDecl{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "a", WriteScope: "b"}) || m.Reviews[1].HalfStars || m.Reviews[1].Scale != 10 {
		t.Errorf("%+v", m.Reviews)
	}
}

func jsonUnmarshal(raw string, v any) error { return json.Unmarshal([]byte(raw), v) }
