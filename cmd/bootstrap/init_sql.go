package bootstrap

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// DefaultInitSQLPath is the conventional path documented for operators (not auto-run).
const DefaultInitSQLPath = "scripts/sql/init.sql"

// ResolveInitSQLPath returns the path only when the operator passed -init-sql explicitly.
// An existing scripts/sql/init.sql on disk does NOT trigger execution by itself.
func ResolveInitSQLPath(flagPath string) string {
	return strings.TrimSpace(flagPath)
}

// RunInitSQLFromPath executes one .sql file or all .sql files in a directory (sorted by name).
func RunInitSQLFromPath(db *gorm.DB, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return RunInitSQL(db, path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(path, e.Name()))
	}
	sort.Strings(files)
	for _, f := range files {
		if err := RunInitSQL(db, f); err != nil {
			return err
		}
	}
	return nil
}
