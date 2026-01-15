package logger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Filename   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxAge     int    `mapstructure:"max_age"`
	MaxBackups int    `mapstructure:"max_backups"`
	Daily      bool   `mapstructure:"daily"`
}

var Lg *zap.Logger

func init() {
	initDefaultLogger()
}

// initDefaultLogger 初始化默认的 logger
func initDefaultLogger() {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	logger, err := config.Build(zap.AddCaller())
	if err != nil {
		Lg = zap.NewNop()
		return
	}

	Lg = logger
	zap.ReplaceGlobals(Lg)
}

// Init init logger
func Init(cfg *LogConfig, mode string) (err error) {
	writeSyncer := getLogWriter(cfg.Filename, cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge, cfg.Daily)
	encoder := getEncoder()
	var l = new(zapcore.Level)
	err = l.UnmarshalText([]byte(cfg.Level))
	if err != nil {
		return
	}
	var core zapcore.Core
	if mode == "dev" || mode == "development" {
		consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // 启用色彩编码
		consoleEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		consoleEncoderConfig.TimeKey = "time"
		consoleEncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
		consoleEncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString("\x1b[90m" + t.Format("2006-01-02 15:04:05.000") + "\x1b[0m")
		}
		consoleEncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			var levelColor = map[zapcore.Level]string{
				zapcore.DebugLevel:  "\x1b[35m", // 紫色
				zapcore.InfoLevel:   "\x1b[36m", // 青色
				zapcore.WarnLevel:   "\x1b[33m", // 黄色
				zapcore.ErrorLevel:  "\x1b[31m", // 红色
				zapcore.DPanicLevel: "\x1b[31m", // 红色
				zapcore.PanicLevel:  "\x1b[31m", // 红色
				zapcore.FatalLevel:  "\x1b[31m", // 红色
			}
			color, ok := levelColor[l]
			if !ok {
				color = "\x1b[0m"
			}
			enc.AppendString(color + "[" + l.CapitalString() + "]\x1b[0m")
		}
		consoleEncoderConfig.EncodeCaller = func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString("\x1b[90m" + caller.TrimmedPath() + "\x1b[0m")
		}
		consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
		highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		})
		lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl < zapcore.ErrorLevel
		})

		core = zapcore.NewTee(
			zapcore.NewCore(encoder, writeSyncer, l),
			zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), lowPriority),
			zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stderr), highPriority),
		)
	} else {
		core = zapcore.NewCore(encoder, writeSyncer, l)
	}

	Lg = zap.New(core, zap.AddCaller())

	zap.ReplaceGlobals(Lg)

	Info("logger initialized successfully")
	return
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	return zapcore.NewJSONEncoder(encoderConfig)
}

func getLogWriter(filename string, maxSize, maxBackup, maxAge int, daily bool) zapcore.WriteSyncer {
	if daily {
		ext := filepath.Ext(filename)
		base := filename[:len(filename)-len(ext)]
		dateStr := time.Now().Format("2006-01-02")
		filename = base + "-" + dateStr + ext
	}

	lumberJackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackup,
		MaxAge:     maxAge,
		LocalTime:  true, // 使用本地时间
	}
	return zapcore.AddSync(lumberJackLogger)
}

// Info common info logger
func Info(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Info(msg, fields...)
}

// Warn common warn logger
func Warn(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Warn(msg, fields...)
}

// Error common error logger
func Error(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Error(msg, fields...)
}

// Debug common debug logger
func Debug(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Debug(msg, fields...)
}

// Fatal common fatal logger
func Fatal(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Fatal(msg, fields...)
}

// Panic common panic logger
func Panic(msg string, fields ...zap.Field) {
	if Lg == nil {
		initDefaultLogger()
	}
	Lg.Panic(msg, fields...)
}

// Sync 刷新缓冲区
func Sync() {
	if Lg != nil {
		_ = Lg.Sync()
	}
}
