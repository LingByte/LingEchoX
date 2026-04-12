package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecoveryMiddleware_WithPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.GET("/test", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Check that error was logged
	logs := recorded.All()
	assert.Greater(t, len(logs), 0)
	assert.Equal(t, "Panic recovered", logs[0].Message)
}

func TestRecoveryMiddleware_WithPanicError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.GET("/test", func(c *gin.Context) {
		panic(assert.AnError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Check that error was logged
	logs := recorded.All()
	assert.Greater(t, len(logs), 0)
}

func TestRecoveryMiddleware_WithPanicString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.POST("/test", func(c *gin.Context) {
		panic("string panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Check that error was logged with path and method
	logs := recorded.All()
	if len(logs) > 0 {
		logEntry := logs[0]

		// Check log fields
		fields := make(map[string]interface{})
		for _, field := range logEntry.Context {
			fields[field.Key] = field.Interface
		}

		assert.Contains(t, fields, "path")
		assert.Contains(t, fields, "method")
		assert.Contains(t, fields, "error")
		assert.Contains(t, fields, "stack")
	}
}

func TestRecoveryMiddleware_WithPanicInt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, recorded := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.GET("/test", func(c *gin.Context) {
		panic(42)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Check that error was logged
	logs := recorded.All()
	assert.Greater(t, len(logs), 0)
}

func TestRecoveryMiddleware_ResponseBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, _ := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.GET("/test", func(c *gin.Context) {
		panic("test panic message")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Internal server error")
	assert.Contains(t, w.Body.String(), "test panic message")
}
