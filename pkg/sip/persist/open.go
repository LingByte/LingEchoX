package persist

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/utils"
)

// OpenStores selects SIP persistence backends:
//   - SIP_PERSIST=json|file → JSON files under SIP_DATA_DIR (default ./data), no database.
//   - otherwise → GORM using GlobalConfig.Database (same as cmd/server after config.Load).
func OpenStores(logWriter io.Writer) (Stores, error) {
	mode := strings.ToLower(strings.TrimSpace(utils.GetEnv("SIP_PERSIST")))
	if mode == "json" {
		dir := strings.TrimSpace(utils.GetEnv("SIP_DATA_DIR"))
		if dir == "" {
			dir = "data"
		}
		if !filepath.IsAbs(dir) {
			if wd, err := filepath.Abs(dir); err == nil {
				dir = wd
			}
		}
		return NewJSONStores(dir)
	}
	db, err := utils.InitDatabase(logWriter, config.GlobalConfig.Database.Driver, config.GlobalConfig.Database.DSN)
	if err != nil {
		return Stores{}, err
	}
	return NewGORMStores(db)
}
