package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	return db
}

func TestDefaultOperationLogConfig(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.NotNil(t, config)
	assert.True(t, config.Enabled)
	assert.False(t, config.LogQueries)
	assert.Contains(t, config.ImportantPatterns["auth"], "/api/auth/login")
	assert.Contains(t, config.UnimportantPostPaths, "/api/auth/refresh")
	assert.Contains(t, config.SystemInternalPaths, "/api/system/")
}

func TestOperationLogConfig_ShouldLogOperation_Disabled(t *testing.T) {
	config := DefaultOperationLogConfig()
	config.Enabled = false
	assert.False(t, config.ShouldLogOperation("POST", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_GET_NotLogQueries(t *testing.T) {
	config := DefaultOperationLogConfig()
	config.LogQueries = false
	assert.False(t, config.ShouldLogOperation("GET", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_GET_LogQueries(t *testing.T) {
	config := DefaultOperationLogConfig()
	config.LogQueries = true
	// GET requests are not logged even if LogQueries is true
	// because ShouldLogOperation only logs write operations (POST, PUT, DELETE, PATCH)
	assert.False(t, config.ShouldLogOperation("GET", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_DELETE(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.ShouldLogOperation("DELETE", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_POST_Important(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.ShouldLogOperation("POST", "/api/auth/login"))
}

func TestOperationLogConfig_ShouldLogOperation_POST_Unimportant(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.False(t, config.ShouldLogOperation("POST", "/api/auth/refresh"))
}

func TestOperationLogConfig_ShouldLogOperation_PUT(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.ShouldLogOperation("PUT", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_PUT_SystemInternal(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.False(t, config.ShouldLogOperation("PUT", "/api/system/test"))
}

func TestOperationLogConfig_ShouldLogOperation_PATCH(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.ShouldLogOperation("PATCH", "/api/test"))
}

func TestOperationLogConfig_ShouldLogOperation_OPTIONS(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.False(t, config.ShouldLogOperation("OPTIONS", "/api/test"))
}

func TestOperationLogConfig_isImportantOperation(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.isImportantOperation("/api/auth/login", "POST"))
	assert.False(t, config.isImportantOperation("/api/test", "GET"))
}

func TestOperationLogConfig_isPostOperationImportant(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.isPostOperationImportant("/api/test"))
	assert.False(t, config.isPostOperationImportant("/api/auth/refresh"))
}

func TestOperationLogConfig_isSystemInternalOperation(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.True(t, config.isSystemInternalOperation("/api/system/test"))
	assert.False(t, config.isSystemInternalOperation("/api/test"))
}

func TestOperationLogConfig_GetOperationDescription_ExactMatch(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "User login", config.GetOperationDescription("POST", "/api/auth/login"))
}

func TestOperationLogConfig_GetOperationDescription_PatternMatch(t *testing.T) {
	config := DefaultOperationLogConfig()
	desc := config.GetOperationDescription("POST", "/api/auth/login/extra")
	assert.Contains(t, desc, "login")
}

func TestOperationLogConfig_GetOperationDescription_Default_DELETE(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "Delete operation", config.GetOperationDescription("DELETE", "/api/unknown"))
}

func TestOperationLogConfig_GetOperationDescription_Default_POST(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "Create operation", config.GetOperationDescription("POST", "/api/unknown"))
}

func TestOperationLogConfig_GetOperationDescription_Default_PUT(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "Update operation", config.GetOperationDescription("PUT", "/api/unknown"))
}

func TestOperationLogConfig_GetOperationDescription_Default_PATCH(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "Partial update operation", config.GetOperationDescription("PATCH", "/api/unknown"))
}

func TestOperationLogConfig_GetOperationDescription_Default_Other(t *testing.T) {
	config := DefaultOperationLogConfig()
	assert.Equal(t, "User operation", config.GetOperationDescription("GET", "/api/unknown"))
}

func TestOperationLogMiddleware_NoDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(OperationLogMiddleware())
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOperationLogMiddleware_InvalidDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, "not-a-db")
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOperationLogMiddleware_NoUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOperationLogMiddleware_WithUserInfo_Struct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set(constants.UserField, &UserInfo{ID: 1, DisplayName: "test"})
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.Header.Set("User-Agent", "test-agent")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Wait for async operation
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserInfo_Map(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set(constants.UserField, map[string]interface{}{
			"id":           uint(1),
			"display_name": "test",
		})
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserInfo_Map_ID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set(constants.UserField, map[string]interface{}{
			"ID":          uint(1),
			"DisplayName": "test",
		})
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserInfo_Map_Username(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set(constants.UserField, map[string]interface{}{
			"user_id":  uint(1),
			"username": "test",
		})
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserID_Context(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set("user_id", uint(1))
		c.Set("username", "test")
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserID_Int(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set("user_id", int(1))
		c.Set("display_name", "test")
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_WithUserID_Int64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set("user_id", int64(1))
		c.Set("display_name", "test")
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	time.Sleep(100 * time.Millisecond)
}

func TestOperationLogMiddleware_NotImportantOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constants.DbField, db)
		c.Set(constants.UserField, &UserInfo{ID: 1, DisplayName: "test"})
		c.Next()
	})
	r.Use(OperationLogMiddleware())
	r.POST("/api/auth/refresh", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOperationLog_TableName(t *testing.T) {
	log := OperationLog{}
	assert.Equal(t, "operation_logs", log.TableName())
}

func TestCreateOperationLog(t *testing.T) {
	db := setupTestDB()
	db.AutoMigrate(&OperationLog{})

	err := CreateOperationLog(db, 1, "test", "POST", "/api/test", "test operation", "127.0.0.1", "test-agent", "http://test.com", "test-device", "test-browser", "test-os", "test-location", "POST")
	assert.NoError(t, err)

	var log OperationLog
	db.First(&log, "user_id = ?", 1)
	assert.Equal(t, uint(1), log.UserID)
	assert.Equal(t, "test", log.Username)
	assert.Equal(t, "POST", log.Action)
}

func TestGetUserInfo_UserInfoStruct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(constants.UserField, &UserInfo{ID: 1, DisplayName: "test"})

	info := getUserInfo(c)
	assert.NotNil(t, info)
	assert.Equal(t, uint(1), info.ID)
	assert.Equal(t, "test", info.DisplayName)
}

func TestGetUserInfo_Map(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(constants.UserField, map[string]interface{}{
		"id":           uint(1),
		"display_name": "test",
	})

	info := getUserInfo(c)
	assert.NotNil(t, info)
	assert.Equal(t, uint(1), info.ID)
}

func TestGetUserInfo_NotExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := getUserInfo(c)
	assert.Nil(t, info)
}

func TestGetUserInfo_ContextUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Set user_id first, then username
	c.Set("user_id", uint(1))
	c.Set("username", "test")

	info := getUserInfo(c)
	// getUserInfo requires constants.UserField to be set, not just user_id
	// So this will return nil
	assert.Nil(t, info)
}

func TestGetUserInfo_ContextUserID_WithUserField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Set user_id in context directly
	c.Set("user_id", uint(1))
	c.Set("display_name", "test")

	info := getUserInfo(c)
	// Without constants.UserField, it will try to get from context
	// But it needs constants.UserField to be set first
	assert.Nil(t, info)
}
