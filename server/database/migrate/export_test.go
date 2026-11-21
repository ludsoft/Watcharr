package migrate

// Test only: expose migrations to the external migrate_test package (which
// can import testutil without an import cycle).
var (
	ForkMigrations = forkMigrations
	AllMigrations  = append(migrations, forkMigrations...)
)
