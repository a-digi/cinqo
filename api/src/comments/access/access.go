// Package access decides who may do what with comments. The storage engine (package
// comments) knows nothing about users or scopes; this package knows which entity
// types an owner has registered, which scopes guard them, and the rules for reading,
// adding, editing and deleting. Owners are the system (types registered in Go) and
// installed tools (types declared in their manifest and cached at install). See
// plan/ai/comments/comments-engine.md.
package access

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/a-digi/cinqo/src/comments"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

var (
	// ErrNotFound: the owner or entity type is unknown, the tool is disabled, or the
	// caller may not even read it. The same answer for all of these, so nobody can
	// probe which types or tools exist.
	ErrNotFound = errors.New("comments are not available here")
	// ErrForbidden: the caller may read these comments but not do what they asked.
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

// EntityType is one kind of commentable entity of an owner and the scopes that
// guard its comments.
type EntityType struct {
	Owner      string
	Name       string
	ReadScope  string
	WriteScope string
}

// Action is what a caller wants to do to an entity's comments.
type Action int

const (
	// Read: list the comments. Needs the read scope.
	Read Action = iota
	// Create: add a comment. Needs the write scope.
	Create
)

// Source finds one owner's entity type. ok is false when the owner has no such type.
type Source interface {
	Lookup(owner, entityType string) (t EntityType, ok bool, err error)
}

// SystemTypes holds the entity types the system itself registers in Go. Safe for
// concurrent use.
type SystemTypes struct {
	mu    sync.RWMutex
	types map[string]EntityType
}

func NewSystemTypes() *SystemTypes {
	return &SystemTypes{types: map[string]EntityType{}}
}

// Register adds a system entity type (for example "media", guarded by
// cinqo:super:admin). It fails on a bad name, a missing scope or a duplicate.
func (s *SystemTypes) Register(name, readScope, writeScope string) error {
	if !comments.ValidEntityType(name) {
		return fmt.Errorf("system comment type %q is not a valid entity type name", name)
	}
	if readScope == "" || writeScope == "" {
		return fmt.Errorf("system comment type %q needs a read and a write scope", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.types[name]; dup {
		return fmt.Errorf("system comment type %q is already registered", name)
	}
	s.types[name] = EntityType{Owner: comments.SystemOwner, Name: name, ReadScope: readScope, WriteScope: writeScope}
	return nil
}

// Lookup implements Source for the system owner only.
func (s *SystemTypes) Lookup(owner, entityType string) (EntityType, bool, error) {
	if owner != comments.SystemOwner {
		return EntityType{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.types[entityType]
	return t, ok, nil
}

// ToolTypes finds the entity types installed, enabled tools declared in their
// manifest. It reads the cached declarations on every call, so an install, update,
// enable, disable or uninstall takes effect without a restart.
type ToolTypes struct {
	db *sql.DB
}

func NewToolTypes(db *sql.DB) *ToolTypes { return &ToolTypes{db: db} }

// Lookup implements Source. The system owner name never resolves to a tool, even
// if a tool were installed under that slug.
func (t *ToolTypes) Lookup(owner, entityType string) (EntityType, bool, error) {
	if owner == comments.SystemOwner {
		return EntityType{}, false, nil
	}
	scopes, err := tool_query.NewToolCommentTypeQueryRepo(t.db).FindEnabled(owner, entityType)
	if errors.Is(err, sql.ErrNoRows) {
		return EntityType{}, false, nil
	}
	if err != nil {
		return EntityType{}, false, err
	}
	return EntityType{Owner: owner, Name: entityType, ReadScope: scopes.ReadScope, WriteScope: scopes.WriteScope}, true, nil
}

// Resolver finds an owner's entity type across its sources and applies the rules.
type Resolver struct {
	sources []Source
}

// NewResolver consults the sources in order; the first that knows the type wins.
func NewResolver(sources ...Source) *Resolver { return &Resolver{sources: sources} }

// Find returns the entity type an owner has registered, with no permission check —
// for callers that are authenticated as the owner itself (a tool purging its own
// comments), not as a user. ok is false when the owner has no such type.
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

// Authorize returns the entity type if the caller may perform the action on its
// comments. ErrNotFound when the type does not exist for that owner or the caller
// cannot read it (a caller without the read scope learns nothing); ErrForbidden when
// they can read but, for Create, lack the write scope. Any other error is a failure
// to look the type up.
func (r *Resolver) Authorize(caller Caller, owner, entityType string, action Action) (EntityType, error) {
	var found EntityType
	var ok bool
	for _, s := range r.sources {
		t, has, err := s.Lookup(owner, entityType)
		if err != nil {
			return EntityType{}, err
		}
		if has {
			found, ok = t, true
			break
		}
	}
	if !ok || !caller.has(found.ReadScope) {
		return EntityType{}, ErrNotFound
	}
	if action == Create && !caller.has(found.WriteScope) {
		return EntityType{}, ErrForbidden
	}
	return found, nil
}

// CanEdit: only the author, and only while they still hold the write scope.
// (Admins may delete anyone's comment but do not rewrite other people's words.)
func CanEdit(t EntityType, caller Caller, c comments.Comment) bool {
	return caller.UserID != "" && c.AuthorID == caller.UserID && caller.has(t.WriteScope)
}

// CanDelete: the author (while holding the write scope), or an administrator.
func CanDelete(t EntityType, caller Caller, c comments.Comment) bool {
	return caller.isAdmin() || CanEdit(t, caller, c)
}
