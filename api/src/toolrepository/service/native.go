package service

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
)

// SyncNative makes the table match the configured address of Cinqo's built-in
// repository (see nativeconfig). It never touches the network: a repository that
// has not been fetched yet has status "never" and is loaded when the page is
// opened. url is the normalised link, or empty for "no native repository".
//
//	url empty                                  -> the native row, if any, becomes ordinary
//	no native row, link not in the table        -> insert it
//	no native row, link already added by hand   -> adopt that row
//	native row with another link                -> point it at the new link and reset it,
//	                                               or adopt a hand-added row that already has it
//	native row with this link                   -> nothing
func (s *Service) SyncNative(url string) error {
	native, err := s.query.FindNative()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	hasNative := err == nil

	if url == "" {
		if hasNative {
			return s.persistent.ClearNative()
		}
		return nil
	}
	if hasNative && native.URL == url {
		return nil
	}

	existing, err := s.query.FindByURL(url)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		return s.persistent.MakeNative(existing.ID)
	}

	name := displayName(&catalog.Catalog{}, url)
	if hasNative {
		return s.persistent.ResetNativeURL(native.ID, url, name)
	}
	return s.persistent.InsertNative(&repo_entity.Repository{ID: uuid.NewString(), URL: url, Name: name})
}
