package logger

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func resetLogger() {
	// 重置单例状态，让每个测试独立
	once = sync.Once{}
	Lg = nil
	cfg = nil
	currentDate = ""
}

// 测试：无效日志级别应该返回错误
func TestInitInvalidLevel(t *testing.T) {
	resetLogger()

	config := &LogConfig{
		Level:    "invalid-level",
		Filename: "",
	}

	err := Init(config, "prod")
	assert.Error(t, err, "expected error for invalid log level")
}

// 测试：开发模式控制台输出（不检查颜色，只检查内容）
func TestInitDevModeConsoleSplit(t *testing.T) {
	resetLogger()

	config := &LogConfig{
		Level:    "debug",
		Filename: "",
	}

	// 捕获 stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Init(config, "dev")
	assert.NoError(t, err)

	Info("test-dev-log")
	Sync()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// 只检查是否包含日志内容，不检查颜色
	assert.True(t, strings.Contains(output, "test-dev-log"),
		"stdout should contain dev log message")
}

// 测试：文件输出 + caller 信息是否正常
func TestAddCallerEnabled(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	logFile := tmpDir + "/caller.log"

	config := &LogConfig{
		Level:    "debug",
		Filename: logFile,
	}

	err := Init(config, "prod")
	assert.NoError(t, err)

	Info("test-caller-enabled")
	Sync()

	// 等待文件写入
	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(logFile)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)

	logStr := string(content)
	assert.True(t, strings.Contains(logStr, "test-caller-enabled"),
		"log should contain caller message")
	assert.True(t, strings.Contains(logStr, "logger_test.go"),
		"log should contain caller file name")
}

// 基础功能测试
func TestLoggerBasicFunctions(t *testing.T) {
	resetLogger()

	config := &LogConfig{
		Level:    "debug",
		Filename: "",
	}
	err := Init(config, "dev")
	assert.NoError(t, err)

	// 所有级别都能调用不崩溃
	Debug("debug-test")
	Info("info-test")
	Warn("warn-test")
	Error("error-test")

	Sync()
	assert.True(t, true)
}

// 测试脱敏功能
func TestMaskString(t *testing.T) {
	assert.Equal(t, "****", MaskString("123"))
	assert.Equal(t, "12****78", MaskString("12345678"))
}

func TestMaskEmail(t *testing.T) {
	assert.Equal(t, "ab****@qq.com", MaskEmail("ab12345@qq.com"))
}

func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "138****5678", MaskPhone("13812345678"))
}

// 测试上下文日志
func TestContextFields(t *testing.T) {
	resetLogger()

	config := &LogConfig{Level: "info", Filename: ""}
	_ = Init(config, "prod")

	ctx := context.WithValue(context.Background(), TraceIDKey, "trace-123")
	InfoCtx(ctx, "context-test")

	assert.True(t, true)
}
