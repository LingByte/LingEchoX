package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
)

// LogConfigInfo Print global configuration information
func LogConfigInfo() {
	logger.Info("system config load finished")
	logger.Info("global config",
		zap.String("server_name", config.GlobalConfig.Server.Name),
		zap.String("server_desc", config.GlobalConfig.Server.Desc),
		zap.String("server_logo", config.GlobalConfig.Server.Logo),
		zap.String("server_url", config.GlobalConfig.Server.URL),
		zap.String("server_terms_url", config.GlobalConfig.Server.TermsURL),
		zap.String("mode", config.GlobalConfig.Server.Mode),
	)

	logger.Info("base config",
		zap.Int64("machine_id", config.GlobalConfig.MachineID),
		zap.String("addr", config.GlobalConfig.Server.Addr),
		zap.String("db_driver", config.GlobalConfig.Database.Driver),
		zap.String("dsn", config.GlobalConfig.Database.DSN),
	)

	logger.Info("log config",
		zap.String("log_level", config.GlobalConfig.Log.Level),
		zap.String("log_filename", config.GlobalConfig.Log.Filename),
		zap.Int("log_max_size", config.GlobalConfig.Log.MaxSize),
		zap.Int("log_max_age", config.GlobalConfig.Log.MaxAge),
		zap.Int("log_max_backups", config.GlobalConfig.Log.MaxBackups),
	)
}

// PrintBannerFromFile Read file and print, auto-generate if file doesn't exist
func PrintBannerFromFile(filename string, defaultText string) error {
	// Ensure banner file exists, generate if it doesn't
	if err := EnsureBannerFile(filename, defaultText); err != nil {
		return fmt.Errorf("failed to ensure banner file: %w", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	colors := []string{
		"\x1b[38;5;17m",
		"\x1b[38;5;18m",
		"\x1b[38;5;19m",
		"\x1b[38;5;20m",
		"\x1b[38;5;21m",
		"\x1b[38;5;26m",
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		color := colors[i%len(colors)]
		fmt.Println(color + line + "\x1b[0m")
	}
	return nil
}
