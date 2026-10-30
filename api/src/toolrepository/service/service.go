// Package service adds, refreshes, lists and removes tool repositories. The
// HTTP handlers stay thin; every rule lives here so it can be tested without a
// server.
package service

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	repo_persistent "github.com/a-digi/cinqo/src/toolrepository/repository/persistent"
	repo_query "github.com/a-digi/cinqo/src/toolrepository/repository/query"
)

const (
	// MaxRepositories bounds how many repositories can be registered.
	MaxRepositories = 20

	maxStoredError = 1000
)

var (
	ErrNotFound      = errors.New("repository not found")
	ErrDuplicate     = errors.New("this repository is already added")
	ErrTooMany       = errors.New("too many repositories")
	ErrInvalidLink   = errors.New("invalid repository link")
	ErrFetchFailed   = errors.New("the repository could not be loaded")
	ErrInvalidFile   = errors.New("the repository file is not valid")
	ErrNativeRemoval = errors.New("the built-in repository cannot be removed")
	errLinkNotAFile  = "this link must point to the JSON file"
)

// Error carries a user-facing reason together with its category.
type Error struct {
	Kind    error
	Message string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Kind }

// Fetcher downloads a repository file; *fetch.Fetcher in production.
type Fetcher interface {
	Get(ctx context.Context, rawURL string, maxBytes int64, policy fetch.LocalPolicy) (*fetch.Result, error)
}

type Service struct {
	db         *sql.DB
	query      *repo_query.RepositoryQueryRepo
	persistent *repo_persistent.RepositoryPersistentRepo
	fetcher    Fetcher
	downloader Fetcher
	now        func() time.Time
}

func New(db *sql.DB, fetcher Fetcher) *Service {
	return &Service{
		db:         db,
		query:      repo_query.NewRepositoryQueryRepo(db),
		persistent: repo_persistent.NewRepositoryPersistentRepo(db),
		fetcher:    fetcher,
		downloader: fetcher,
		now:        time.Now,
	}
}

func (s *Service) List() ([]*repo_entity.Repository, error) { return s.query.List() }

// NormalizeLink validates the typed link: absolute, https (http only for a local
// address), no credentials, and pointing at a file rather than a folder. The
// link is the repository's identity, so every spelling of the same address maps
// to one string: scheme and host in lower case, no default port, no trailing dot
// on the host, no fragment, no empty "?". The path and query are left exactly as
// typed, because a server may treat them as case-sensitive.
func NormalizeLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &Error{ErrInvalidLink, "link is required"}
	}
	if u, err := url.Parse(raw); err != nil || !u.IsAbs() {
		return "", &Error{ErrInvalidLink, "the link must be an absolute http(s) address"}
	}
	link, err := catalog.ResolvePackageURL("", raw)
	if err != nil {
		return "", &Error{ErrInvalidLink, "the link " + err.Error()}
	}
	u, _ := url.Parse(link)
	if u.Path == "" || strings.HasSuffix(u.Path, "/") || path.Base(u.Path) == "." {
		return "", &Error{ErrInvalidLink, errLinkNotAFile}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	u.Host = host
	if port != "" {
		u.Host = host + ":" + port
	}
	u.ForceQuery = false
	return u.String(), nil
}

// Add fetches and validates the file at link and stores the repository only if
// the whole file is valid.
func (s *Service) Add(ctx context.Context, rawLink string) (*repo_entity.Repository, error) {
	link, err := NormalizeLink(rawLink)
	if err != nil {
		return nil, err
	}
	if _, err := s.query.FindByURL(link); err == nil {
		return nil, &Error{ErrDuplicate, "this repository is already added"}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	n, err := s.query.CountUserAdded()
	if err != nil {
		return nil, err
	}
	if n >= MaxRepositories {
		return nil, &Error{ErrTooMany, "at most 20 repositories can be added; remove one first"}
	}

	cat, raw, local, err := s.load(ctx, link)
	if err != nil {
		return nil, err
	}
	now := s.stamp()
	repo := &repo_entity.Repository{
		ID:            uuid.NewString(),
		URL:           link,
		Name:          displayName(cat, link),
		Description:   cat.Description,
		CatalogJSON:   string(raw),
		IsLocal:       local,
		LastFetchedAt: now,
		LastStatus:    repo_entity.StatusOK,
	}
	if err := s.persistent.Insert(repo); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, &Error{ErrDuplicate, "this repository is already added"}
		}
		return nil, err
	}
	return s.query.FindByID(repo.ID)
}

// Refresh refetches a repository. A failure keeps the last good catalog and is
// recorded on the repository, which is returned either way.
func (s *Service) Refresh(ctx context.Context, id string) (*repo_entity.Repository, error) {
	repo, err := s.find(id)
	if err != nil {
		return nil, err
	}
	cat, raw, local, loadErr := s.load(ctx, repo.URL)
	now := s.stamp()
	if loadErr != nil {
		msg := loadErr.Error()
		if len(msg) > maxStoredError {
			msg = msg[:maxStoredError]
		}
		if err := s.persistent.RecordError(id, msg, now); err != nil {
			return nil, err
		}
	} else if err := s.persistent.RecordSuccess(id, displayName(cat, repo.URL), cat.Description, string(raw), now, local); err != nil {
		return nil, err
	}
	return s.query.FindByID(id)
}

// Remove deletes the registration only; installed tools are never touched.
func (s *Service) Remove(id string) error {
	repo, err := s.find(id)
	if err != nil {
		return err
	}
	if repo.IsNative {
		return &Error{ErrNativeRemoval, "the built-in repository cannot be removed"}
	}
	return s.persistent.Delete(id)
}

func (s *Service) find(id string) (*repo_entity.Repository, error) {
	repo, err := s.query.FindByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &Error{ErrNotFound, "repository not found"}
	}
	return repo, err
}

func (s *Service) stamp() string { return s.now().UTC().Format("2006-01-02 15:04:05") }

// load downloads and validates the file. A repository on a public address is
// also refused if its packages point at local addresses.
func (s *Service) load(ctx context.Context, link string) (*catalog.Catalog, []byte, bool, error) {
	res, err := s.fetcher.Get(ctx, link, catalog.MaxFileBytes, fetch.LocalByFirstHop)
	if err != nil {
		return nil, nil, false, &Error{ErrFetchFailed, "the repository could not be loaded: " + err.Error()}
	}
	cat, err := catalog.Parse(res.Body, link)
	if err != nil {
		return nil, nil, false, invalidFile(err)
	}
	if !res.Local {
		if probs := catalog.LocalPackageProblems(cat); len(probs) > 0 {
			return nil, nil, false, invalidFile(&catalog.ValidationError{Problems: probs})
		}
	}
	return cat, res.Body, res.Local, nil
}

func invalidFile(err error) error {
	var ve *catalog.ValidationError
	if errors.As(err, &ve) {
		return &Error{ErrInvalidFile, "the repository file is not valid: " + joinProblems(ve.Problems)}
	}
	return &Error{ErrInvalidFile, "the repository file is not valid"}
}

func joinProblems(ps []catalog.Problem) string {
	const shown = 20
	parts := make([]string, 0, shown+1)
	for i, p := range ps {
		if i == shown {
			parts = append(parts, "and "+strconv.Itoa(len(ps)-shown)+" more")
			break
		}
		parts = append(parts, p.String())
	}
	return strings.Join(parts, "; ")
}

// displayName is the file's own name, else the link's host and file name.
func displayName(c *catalog.Catalog, link string) string {
	if n := strings.TrimSpace(c.Name); n != "" {
		return n
	}
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	return u.Host + "/" + path.Base(u.Path)
}
