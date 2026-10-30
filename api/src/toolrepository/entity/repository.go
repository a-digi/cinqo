package entity

// Repository is a registered tool repository: the link to its JSON file plus
// the last file that passed validation, kept verbatim so a failed refresh
// never loses the last good catalog.
type Repository struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// CatalogJSON is the last validated repository file; read by the status
	// computation, never sent to the client as is.
	CatalogJSON string `json:"-"`
	// IsLocal is true when the repository was reached on a local address; its
	// packages are then downloaded with local addresses allowed, a public
	// repository's never.
	IsLocal       bool   `json:"isLocal"`
	LastFetchedAt string `json:"lastFetchedAt"`
	LastStatus    string `json:"lastStatus"`
	LastError     string `json:"lastError"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

const (
	StatusNever = "never"
	StatusOK    = "ok"
	StatusError = "error"
)
