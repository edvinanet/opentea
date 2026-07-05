package repo

import (
	"path/filepath"
	"testing"

	"github.com/oej/opentea/internal/db"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return New(sqlDB)
}
