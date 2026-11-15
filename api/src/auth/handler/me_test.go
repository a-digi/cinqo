package auth_handler

import (
	"net/http/httptest"
	"testing"

	db "github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"

	organisation_entity "github.com/a-digi/cinqo/src/organisation/entity"
)

func TestMeHasTheIdentityOnlyWhenTheUserInfoCookieCameAndTheOrganisationWhenKnown(t *testing.T) {
	scopes := []string{"a"}
	bare := buildMe("u1", nil, scopes, 99, nil)
	if len(bare) != 3 || bare["user_id"] != "u1" || bare["expires_at"] != int64(99) {
		t.Errorf("%v", bare)
	}
	full := buildMe("u1", &userInfoCookie{Email: "e@x.de", Name: "Ann", PreferredUsername: "ann"}, scopes, 99,
		&organisation_entity.Organisation{ID: "o1", Name: "Acme", Status: "suspended"})
	if full["email"] != "e@x.de" || full["name"] != "Ann" || full["preferred_username"] != "ann" ||
		full["organisation_id"] != "o1" || full["organisation_name"] != "Acme" || full["organisation_status"] != "suspended" {
		t.Errorf("%v", full)
	}
	if _, has := bare["organisation_id"]; has {
		t.Error("no organisation read, no organisation fields")
	}
	if _, has := buildMe("u1", nil, scopes, 1, &organisation_entity.Organisation{ID: "o"})["email"]; has {
		t.Error("no cookie, no e-mail")
	}
}

func TestAnOrganisationThatCannotBeReadLeavesTheFieldsOutInsteadOfBreakingTheSignIn(t *testing.T) {
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil), nil)
	if organisationOf(ctx, "u1") != nil {
		t.Error("without a database there is no organisation")
	}
}

// noDatabaseDI is a DI whose database manager is missing (a start-up problem): the sign-in must still answer.
type noDatabaseDI struct{ serverdi.Context }

func (noDatabaseDI) GetDatabaseManager() *db.DatabaseManager { return nil }

func TestAMissingDatabaseManagerLeavesTheOrganisationOut(t *testing.T) {
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil), noDatabaseDI{})
	if organisationOf(ctx, "u1") != nil {
		t.Error("no database manager, no organisation, and no panic")
	}
}
