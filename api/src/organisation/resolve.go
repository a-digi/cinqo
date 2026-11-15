package organisation

import (
	"database/sql"
	"errors"
	"os"
	"strings"

	"github.com/a-digi/cinqo/src/organisation/entity"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
	"github.com/a-digi/cinqo/src/organisation/repository/query"
)

// Header carries the organisation a tool acts for. Only core sets it (the tool proxy and the dashboard widget fetch): any value a caller sent is thrown
// away first. A tool may trust it because its backend listens on 127.0.0.1 behind core.
const Header = "X-Cinqo-Org"

// ActingOrgHeader and CallerToolHeader belong to unattended tool calls (a tool calling another tool with its own service token). A tool names the organisation
// it acts for in ActingOrgHeader; core checks it and answers to the target tool with Header, plus CallerToolHeader (who is calling). A person's request never
// carries either: core deletes both from it.
const (
	ActingOrgHeader  = "X-Cinqo-Acting-Org"
	CallerToolHeader = "X-Cinqo-Caller-Tool"
)

// BackgroundKillSwitchEnv, set to "off" in the environment of the app, stops every unattended tool call at once (the tool proxy checks it on every call; the
// operator's page shows it).
const BackgroundKillSwitchEnv = "CINQO_BACKGROUND"

// BackgroundKillSwitchOn says whether unattended calls are switched off for the whole app.
func BackgroundKillSwitchOn() bool { return os.Getenv(BackgroundKillSwitchEnv) == "off" }

// ErrNoUser is returned for an empty user id.
var ErrNoUser = errors.New("organisation: no user id")

// Of returns the organisation of a user. A user who has none yet is attached to the default organisation first (what every user of an existing install
// already is), so the answer is never "none".
func Of(db *sql.DB, userID string) (*entity.Organisation, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrNoUser
	}
	q := query.NewOrganisationQueryRepo(db)
	// Read first: this runs on every tool request, and a write on each of them would be wasted work.
	if o, err := q.FindByUser(userID); err == nil {
		return o, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := persistent.NewOrganisationPersistentRepo(db).AttachDefault(userID); err != nil {
		return nil, err
	}
	return q.FindByUser(userID)
}
