package migrate_test

import (
	"testing"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/database/migrate"
	"github.com/sbondCo/Watcharr/internal/testutil"
)

func TestForkMigrationOIDCUserType(t *testing.T) {
	testutil.SetupLogging()
	db := testutil.SetupDB(t)

	// An OIDC user created by an older fork version (type 4), and a type 4
	// user that is NOT linked to OIDC (eg: a future upstream type 4).
	oidcUser := entity.User{Username: "oidc", Password: "x", Type: 4, UserServices: []entity.UserServices{
		{Name: entity.UserServiceNameOIDC, ClientID: "https://idp|sub-1"},
	}}
	otherUser := entity.User{Username: "other", Password: "x", Type: 4}
	for _, u := range []*entity.User{&oidcUser, &otherUser} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := migrate.ForkMigrations[0].Up(db); err != nil {
		t.Fatal(err)
	}
	// Running again must be harmless.
	if err := migrate.ForkMigrations[0].Up(db); err != nil {
		t.Fatal(err)
	}

	typeOf := func(id uint) entity.UserType {
		var u entity.User
		if err := db.Select("type").Where("id = ?", id).Take(&u).Error; err != nil {
			t.Fatal(err)
		}
		return u.Type
	}
	if got := typeOf(oidcUser.ID); got != entity.OIDC_USER {
		t.Errorf("oidc user type = %d, want %d", got, entity.OIDC_USER)
	}
	if got := typeOf(otherUser.ID); got != 4 {
		t.Errorf("non oidc type 4 user changed to %d", got)
	}
}

func TestForkMigrationIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range migrate.AllMigrations {
		if seen[m.ID] {
			t.Errorf("duplicate migration id %s", m.ID)
		}
		seen[m.ID] = true
	}
}
