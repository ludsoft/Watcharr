package migrate

import (
	"log/slog"

	"github.com/sbondCo/Watcharr/database/entity"
	"gorm.io/gorm"
)

// Migrations of the ludsoft fork.
//
// Kept apart from upstream's `migrations` (migrations.go) so syncing with
// upstream doesn't conflict, and ran after them. IDs use a `_ludsoft_`
// suffix so they can never clash with upstream migration IDs. Same rules as
// upstream: never change a migration once released, add a new one.
var forkMigrations = []Migration{
	{
		// OIDC users were created with user type 4, which upstream could
		// reuse for something else. Move them to the fork value 99. Only
		// users linked to an OIDC login are touched.
		ID: "202611211200_ludsoft_0001",
		Up: func(tx *gorm.DB) error {
			migID := "202611211200_ludsoft_0001"
			res := tx.Exec(
				`UPDATE users SET type = ?
				 WHERE type = 4
				   AND id IN (SELECT user_id FROM user_services WHERE name = ?)`,
				entity.OIDC_USER,
				entity.UserServiceNameOIDC,
			)
			if res.Error != nil {
				slog.Error("Migration failed!", "mig", migID, "error", res.Error)
				return res.Error
			}
			slog.Info("Migration complete.", "mig", migID, "oidc_users_moved", res.RowsAffected)
			return nil
		},
	},
}
