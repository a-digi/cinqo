package handler

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	db "github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/dashboard"
)

type noDatabaseDI struct{ serverdi.Context }

func (noDatabaseDI) GetDatabaseManager() *db.DatabaseManager { return nil }

func TestACallerWhoseOrganisationCannotBeReadIsLeftAsItIs(t *testing.T) {
	c := dashboard.Caller{UserID: "u1", Token: "t"}
	for name, di := range map[string]serverdi.Context{"no DI": nil, "no database manager": noDatabaseDI{}} {
		ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil), di)
		if got := withOrganisation(ctx, c); got.UserID != "u1" || got.Token != "t" || got.OrgID != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// callerFrom must put the organisation on the caller: without it two organisations of one user would share a cache entry.
func TestCallerFromAddsTheOrganisation(t *testing.T) {
	raw, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "return withOrganisation(reqCtx, dashboard.Caller{UserID: sub, Scopes: scopes, Token: token}), nil") {
		t.Error("callerFrom no longer adds the organisation")
	}
}
