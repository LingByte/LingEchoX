package bootstrap

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/logger"
	sipPersist "github.com/LinByte/VoiceServer/pkg/sip/persist"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Options controls database initialization behavior
type Options struct {
	// InitSQLPath points to a .sql script file (optional); skip if empty
	InitSQLPath string
	// AutoMigrate whether to execute entity migration (default true)
	AutoMigrate bool
	// SeedNonProd whether to write default configuration in non-production environments (default true)
	SeedNonProd bool
}

// SetupDatabase unified entry: connect database -> run initialization SQL -> migrate entities -> (non-production) write default configuration
func SetupDatabase(logWriter io.Writer, opts *Options) (*gorm.DB, error) {
	if opts == nil {
		opts = &Options{AutoMigrate: false, SeedNonProd: false}
	}

	// 1) Connect to database
	db, err := initDBConn(logWriter)
	if err != nil {
		logger.Error("init database failed", zap.Error(err))
		return nil, err
	}

	initPath := ResolveInitSQLPath(opts.InitSQLPath)

	// 2) -init: GORM AutoMigrate + cmd/bootstrap/migrations/*.sql (indexes only, no tenant dump)
	if opts.AutoMigrate {
		if err := RunMigrations(db); err != nil {
			logger.Error("migration failed", zap.Error(err))
			return nil, err
		}
		if err := runPostMigrateSQL(db, "cmd/bootstrap/migrations"); err != nil {
			logger.Warn("post-migrate sql failed", zap.Error(err))
		}
		logger.Info("migration success",
			zap.String("database", config.GlobalConfig.Database.Driver),
			zap.String("dsn", config.GlobalConfig.Database.DSN),
		)
	}

	// 3) Permission catalog (required before init-sql role bindings; also runs with -seed)
	if opts.SeedNonProd || initPath != "" {
		if err := models.SyncPermissionCatalog(db); err != nil {
			logger.Error("sync permission catalog failed", zap.Error(err))
			return nil, err
		}
	}

	// 4) -init-sql ONLY: optional tenant/trunk dump (scripts/sql/init.sql). Separate from migrations/.
	if initPath != "" {
		logger.Info("running init sql", zap.String("path", initPath))
		if err := RunInitSQLFromPath(db, initPath); err != nil {
			logger.Error("run init sql failed", zap.String("path", initPath), zap.Error(err))
			return nil, err
		}
		if err := models.BackfillSystemTenantAdminPermissions(db, "init-sql"); err != nil {
			logger.Error("bind tenant admin permissions failed", zap.Error(err))
			return nil, err
		}
		logger.Info("tenant admin permissions backfilled from permission catalog")
	}

	// 5) Non-production: site config + default platform admin (+ re-backfill if seed runs after init-sql)
	if opts.SeedNonProd {
		service := SeedService{db: db}
		if err := service.SeedAll(); err != nil {
			logger.Error("seed failed", zap.Error(err))
			return nil, err
		}
	}

	logger.Info("system bootstrap - database is initialization complete")
	return db, nil
}

// initDBConn creates *gorm.DB based on global configuration
func initDBConn(logWriter io.Writer) (*gorm.DB, error) {
	dbDriver := config.GlobalConfig.Database.Driver
	dsn := config.GlobalConfig.Database.DSN
	return utils.InitDatabase(logWriter, dbDriver, dsn)
}

// RunInitSQL executes SQL statements from a local .sql file segment by segment (split by semicolon ;), idempotent scripts should use IF NOT EXISTS in SQL for protection
func RunInitSQL(db *gorm.DB, sqlFilePath string) error {
	f, err := os.Open(sqlFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var (
		sb      strings.Builder
		scanner = bufio.NewScanner(f)
	)
	// Relax token limit (long lines)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		// Ignore comment lines (starting with --) and empty lines
		if trim == "" || strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "#") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		// Use ; as statement terminator (simple splitting, suitable for most scenarios)
		if strings.HasSuffix(trim, ";") {
			stmt := strings.TrimSpace(sb.String())
			sb.Reset()
			if stmt != "" {
				if err := db.Exec(stmt).Error; err != nil {
					return err
				}
			}
		}
	}
	// Handle remaining content at end of file without semicolon
	rest := strings.TrimSpace(sb.String())
	if rest != "" {
		if err := db.Exec(rest).Error; err != nil {
			return err
		}
	}
	return scanner.Err()
}

// runPostMigrateSQL runs cmd/bootstrap/migrations/*.sql after GORM AutoMigrate (-init flag).
// Not the same as -init-sql (operator tenant seed under scripts/sql/).
func runPostMigrateSQL(db *gorm.DB, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 迁移目录不存在，跳过
			return nil
		}
		return err
	}

	// 按文件名排序执行迁移脚本
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(migrationsDir, entry.Name())
		logger.Info("executing migration script", zap.String("file", filePath))
		if err := RunInitSQL(db, filePath); err != nil {
			logger.Error("migration script failed", zap.String("file", filePath), zap.Error(err))
			return err
		}
	}

	return nil
}

// RunMigrations executes entity migration
func RunMigrations(db *gorm.DB) error {
	if db == nil {
		return errors.New("db is nil")
	}
	return utils.MakeMigrates(db, []any{
		&utils.Config{},
		&sipPersist.SIPUser{},
		&sipPersist.SIPCall{},
		&models.ACDPoolTarget{},
		&models.SIPACDTransferOffer{},
		&models.SIPCampaign{},
		&models.SIPCampaignContact{},
		&models.SIPCallAttempt{},
		&models.SIPScriptRun{},
		&models.SIPCampaignEvent{},
		&models.SIPScriptTemplate{},
		&models.Trunk{},
		&models.TrunkNumber{},
		&models.Tenant{},
		&models.TenantGroup{},
		&models.TenantUser{},
		&models.TenantUserGroup{},
		&models.Permission{},
		&models.TenantRole{},
		&models.TenantRolePermission{},
		&models.TenantUserRole{},
		&models.Credential{},
		&models.PlatformAdmin{},
		&models.VoiceTrainingTask{},
		&models.VoiceClone{},
		&models.VoiceSynthesis{},
		&models.VoiceTrainingText{},
		&models.VoiceTrainingTextSegment{},
	})
}

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
