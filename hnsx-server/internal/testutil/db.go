// Package testutil provides shared test helpers for the hnsx-server modules.
package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/db"
)

// OpenTestDB connects to Postgres for integration tests. It skips the test if
// the database is unavailable. Migrations under /go/migrations are applied.
func OpenTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("HNSX_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hnsx:hnsx@localhost:5432/hnsx?sslmode=disable"
	}

	ctx := context.Background()
	database, err := db.Open(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if database.IsNoDB() {
		t.Skip("postgres unavailable (NoDB)")
	}

	wd, _ := os.Getwd()
	migrationsDir := findMigrationsDir(t, wd)
	if err := db.Migrate(ctx, database, migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func findMigrationsDir(t *testing.T, start string) string {
	t.Helper()
	for dir := start; dir != "/"; dir = parent(dir) {
		candidate := dir + "/go/migrations"
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatal("could not find go/migrations from " + start)
	return ""
}

func parent(dir string) string {
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			if i == 0 {
				return "/"
			}
			return dir[:i]
		}
	}
	return "/"
}
