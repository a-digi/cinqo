// Package access decides who may do what with reviews. The storage engine (package review) knows nothing about users or scopes; this
// package knows which entity types an owner has registered, which scale and scopes guard each, and the rules for reading, giving
// and deleting a review. Owners are the system (types registered in Go) and installed tools (types declared in their manifest and
// cached at install). It mirrors api/src/comments/access. See plan/ai/review/review-engine.md.
package access

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/a-digi/cinqo/src/review"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

var (
	// ErrNotFound: the owner or entity type is unknown, the tool is disabled, or the caller may not even read it. The same answer
	// for all of these, so nobody can probe which types or tools exist.
	ErrNotFound = errors.New("reviews are not available here")
	// ErrForbidden: the caller may read these reviews but not do what they asked.
	ErrForbidden = errors.New("not allowed")
)

// AdminScope satisfies every scope check, as everywhere else in Cinqo.
const AdminScope = "cinqo:super:admin"

// Caller is who is acting.
type Caller struct {
	UserID string
	Scopes []string
}

func (c Caller) has(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope || s == AdminScope {
			return true
		}
	}
	return false
}

func (c Caller) isAdmin() bool {
	for _, s := range c.Scopes {
		if s == AdminScope {
			return true
		}
	}
	return false
}

// EntityType is one kind of reviewable entity of an owner: the scale its ratings use and the scopes that guard its reviews.
type EntityType struct {
	Owner      string
	Name       string
	Scale      int
	HalfStars  bool
	ReadScope  string
	WriteScope string
}

// Action is what a caller wants to do with an entity's reviews.
type Action int

const (
	// Read: see the summary and the reviews. Needs the read scope.
	Read Action = iota
	// Write: give, change or remove one's own review. Needs the write scope.
	Write
)

// Source finds one owner's entity type. ok is false when the owner has no such type.
type Source interface {
	Lookup(owner, entityType string) (t EntityType, ok bool, err error)
}

// SystemTypes holds the entity types the system itself registers in Go. Safe for concurrent use.
type SystemTypes struct {
	mu    sync.RWMutex
	types map[string]EntityType
}

func NewSystemTypes() *SystemTypes {
	return &SystemTypes{types: map[string]EntityType{}}
}

// Register adds a system entity type (for example "media", guarded by cinqo:super:admin). It fails on a bad name, a bad scale, a
// missing scope or a duplicate.
func (s *SystemTypes) Register(name string, scale int, halfStars bool, readScope, writeScope string) error {
	if !review.ValidEntityType(name) {
		return fmt.Errorf("system review type %q is not a valid entity type name", name)
	}
	if !review.ValidScale(scale) {
		return fmt.Errorf("system review type %q: scale %d must be 3, 5 or 10", name, scale)
	}
	if readScope == "" || writeScope == "" {
		return fmt.Errorf("system review type %q needs a read and a write scope", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.types[name]; dup {
		return fmt.Errorf("system review type %q is already registered", name)
	}
	s.types[name] = EntityType{Owner: review.SystemOwner, Name: name, Scale: scale, HalfStars: halfStars, ReadScope: readScope, WriteScope: writeScope}
	return nil
}

// Lookup implements Source for the system owner only.
func (s *SystemTypes) Lookup(owner, entityType string) (EntityType, bool, error) {
	if owner != review.SystemOwner {
		return EntityType{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.types[entityType]
	return t, ok, nil
}

// ToolTypes finds the entity types installed, enabled tools declared in their manifest. It reads the cached declarations on every
// call, so an install, update, enable, disable or uninstall takes effect without a restart.
type ToolTypes struct {
	db *sql.DB
}

func NewToolTypes(db *sql.DB) *ToolTypes { return &ToolTypes{db: db} }

// Lookup implements Source. The system owner name never resolves to a tool, even if a tool were installed under that slug.
func (t *ToolTypes) Lookup(owner, entityType string) (EntityType, bool, error) {
	if owner == review.SystemOwner {
		return EntityType{}, false, nil
	}
	spec, err := tool_query.NewToolReviewTypeQueryRepo(t.db).FindEnabled(owner, entityType)
	if errors.Is(err, sql.ErrNoRows) {
		return EntityType{}, false, nil
	}
	if err != nil {
		return EntityType{}, false, err
	}
	return EntityType{
		Owner: owner, Name: entityType, Scale: spec.Scale, HalfStars: spec.HalfStars,
		ReadScope: spec.ReadScope, WriteScope: spec.WriteScope,
	}, true, nil
}

// Resolver finds an owner's entity type across its sources and applies the rules.
type Resolver struct {
	sources []Source
}

// NewResolver consults the sources in order; the first that knows the type wins.
func NewResolver(sources ...Source) *Resolver { return &Resolver{sources: sources} }

// Find returns the entity type an owner has registered, with no permission check, for callers that are authenticated as the owner
// itself (a tool purging its own reviews), not as a user. ok is false when the owner has no such type.
func (r *Resolver) Find(owner, entityType string) (t EntityType, ok bool, err error) {
	for _, s := range r.sources {
		t, has, err := s.Lookup(owner, entityType)
		if err != nil {
			return EntityType{}, false, err
		}
		if has {
			return t, true, nil
		}
	}
	return EntityType{}, false, nil
}

// Authorize returns the entity type if the caller may perform the action on its reviews. ErrNotFound when the type does not exist
// for that owner or the caller cannot read it (a caller without the read scope learns nothing); ErrForbidden when they can read but,
// for Write, lack the write scope. Any other error is a failure to look the type up.
func (r *Resolver) Authorize(caller Caller, owner, entityType string, action Action) (EntityType, error) {
	found, ok, err := r.Find(owner, entityType)
	if err != nil {
		return EntityType{}, err
	}
	if !ok || !caller.has(found.ReadScope) {
		return EntityType{}, ErrNotFound
	}
	if action == Write && !caller.has(found.WriteScope) {
		return EntityType{}, ErrForbidden
	}
	return found, nil
}

// CanEdit: only the author, and only while they still hold the write scope (the review's rating and text are theirs; an
// administrator may remove someone's review but never rewrites it).
func CanEdit(t EntityType, caller Caller, r review.Review) bool {
	return caller.UserID != "" && r.AuthorID == caller.UserID && caller.has(t.WriteScope)
}

// CanDelete: the author (while holding the write scope), or an administrator.
func CanDelete(t EntityType, caller Caller, r review.Review) bool {
	return caller.isAdmin() || CanEdit(t, caller, r)
}
