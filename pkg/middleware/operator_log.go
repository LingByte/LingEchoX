package middleware

import (
	"log"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/mssola/user_agent"
	"gorm.io/gorm"
)

// OperationLogConfig represents operation log configuration
type OperationLogConfig struct {
	// Whether to enable operation logging
	Enabled bool
	// Whether to log query operations
	LogQueries bool
	// Important operation patterns
	ImportantPatterns map[string][]string
	// Unimportant POST operations
	UnimportantPostPaths []string
	// System internal operation paths
	SystemInternalPaths []string
	// Operation description mapping
	OperationDescriptions map[string]string
}

// DefaultOperationLogConfig returns default configuration
func DefaultOperationLogConfig() *OperationLogConfig {
	return &OperationLogConfig{
		Enabled:    true,
		LogQueries: false,
		ImportantPatterns: map[string][]string{
			// Authentication-related important operations
			"auth": {
				"/api/auth/login",
				"/api/auth/register",
				"/api/auth/logout",
				"/api/auth/change-password",
				"/api/auth/reset-password",
				"/api/auth/verify-email",
				"/api/auth/two-factor",
			},
			// User profile important operations
			"profile": {
				"/api/auth/update",
				"/api/auth/preferences",
			},
			// Notification important operations
			"notification": {
				"/api/notification/mark-read",
				"/api/notification/delete",
				"/api/notification/clear",
			},
			// Assistant important operations
			"assistant": {
				"/api/assistant/create",
				"/api/assistant/update",
				"/api/assistant/delete",
			},
			// Chat important operations
			"chat": {
				"/api/chat/send",
				"/api/chat/delete",
				"/api/chat/clear",
			},
			// Voice training important operations
			"voice": {
				"/api/voice/training/create",
				"/api/voice/training/update",
				"/api/voice/training/delete",
			},
			// Knowledge base important operations
			"knowledge": {
				"/api/knowledge/create",
				"/api/knowledge/update",
				"/api/knowledge/delete",
			},
			// Group important operations
			"group": {
				"/api/group/create",
				"/api/group/update",
				"/api/group/delete",
				"/api/group/join",
				"/api/group/leave",
			},
			// Credentials important operations
			"credentials": {
				"/api/credentials/create",
				"/api/credentials/update",
				"/api/credentials/delete",
			},
			// File upload important operations
			"upload": {
				"/api/upload",
			},
		},
		UnimportantPostPaths: []string{
			"/api/auth/refresh",      // Token refresh
			"/api/notification/read", // Mark as read (batch operation)
			"/api/chat/typing",       // Typing status
			"/api/voice/heartbeat",   // Voice heartbeat
			"/api/metrics/collect",   // Metrics collection
		},
		SystemInternalPaths: []string{
			"/api/system/",
			"/api/internal/",
			"/api/debug/",
			"/api/test/",
		},
		OperationDescriptions: map[string]string{
			"/api/auth/login":             "User login",
			"/api/auth/logout":            "User logout",
			"/api/auth/register":          "User registration",
			"/api/auth/change-password":   "Change password",
			"/api/auth/reset-password":    "Reset password",
			"/api/auth/update":            "Update profile",
			"/api/auth/preferences":       "Update preferences",
			"/api/auth/two-factor":        "Two-factor authentication",
			"/api/notification/mark-read": "Mark notification as read",
			"/api/notification/delete":    "Delete notification",
			"/api/notification/clear":     "Clear notifications",
			"/api/assistant/create":       "Create assistant",
			"/api/assistant/update":       "Update assistant",
			"/api/assistant/delete":       "Delete assistant",
			"/api/chat/send":              "Send message",
			"/api/chat/delete":            "Delete chat record",
			"/api/voice/training/create":  "Create voice training",
			"/api/voice/training/update":  "Update voice training",
			"/api/voice/training/delete":  "Delete voice training",
			"/api/knowledge/create":       "Create knowledge base",
			"/api/knowledge/update":       "Update knowledge base",
			"/api/knowledge/delete":       "Delete knowledge base",
			"/api/group/create":           "Create group",
			"/api/group/join":             "Join group",
			"/api/group/leave":            "Leave group",
			"/api/upload":                 "File upload",
		},
	}
}

// ShouldLogOperation determines whether to log operation based on configuration
func (config *OperationLogConfig) ShouldLogOperation(method, path string) bool {
	if !config.Enabled {
		return false
	}

	// 1. If not logging queries, skip GET, HEAD, OPTIONS
	if !config.LogQueries && (method == "GET" || method == "HEAD" || method == "OPTIONS") {
		return false
	}

	// 2. Only log write operations (POST, PUT, DELETE, PATCH)
	if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
		return false
	}

	// 3. Check if it's an important operation
	return config.isImportantOperation(path, method)
}

// isImportantOperation determines if operation is important
func (config *OperationLogConfig) isImportantOperation(path, method string) bool {
	// Check if matches important operation patterns
	for _, patterns := range config.ImportantPatterns {
		for _, pattern := range patterns {
			if strings.HasPrefix(path, pattern) {
				return true
			}
		}
	}

	// Importance judgment based on HTTP method
	switch method {
	case "DELETE":
		// Delete operations are usually important
		return true
	case "POST":
		// POST operations need further judgment
		return config.isPostOperationImportant(path)
	case "PUT", "PATCH":
		// Update operations are usually important, but exclude some system internal operations
		return !config.isSystemInternalOperation(path)
	}

	return false
}

// isPostOperationImportant determines if POST operation is important
func (config *OperationLogConfig) isPostOperationImportant(path string) bool {
	// Exclude some unimportant POST operations
	for _, unimportantPath := range config.UnimportantPostPaths {
		if strings.HasPrefix(path, unimportantPath) {
			return false
		}
	}

	// Other POST operations are considered important
	return true
}

// isSystemInternalOperation determines if it's a system internal operation
func (config *OperationLogConfig) isSystemInternalOperation(path string) bool {
	for _, internalPath := range config.SystemInternalPaths {
		if strings.HasPrefix(path, internalPath) {
			return true
		}
	}
	return false
}

// GetOperationDescription gets operation description
func (config *OperationLogConfig) GetOperationDescription(method, path string) string {
	// First try to get exact match from configuration
	if desc, exists := config.OperationDescriptions[path]; exists {
		return desc
	}

	// Pattern matching based on path
	for pattern, desc := range config.OperationDescriptions {
		if strings.Contains(path, pattern) {
			return desc
		}
	}

	// Default description based on HTTP method
	switch method {
	case "DELETE":
		return "Delete operation"
	case "POST":
		return "Create operation"
	case "PUT":
		return "Update operation"
	case "PATCH":
		return "Partial update operation"
	default:
		return "User operation"
	}
}

// UserInfo represents user information for operation logging
type UserInfo struct {
	ID          uint   // User ID
	DisplayName string // User display name
}

// Global configuration instance
var operationLogConfig = DefaultOperationLogConfig()

// OperationLogMiddleware records operation logs
func OperationLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		db, exists := c.Get(constants.DbField)
		if !exists {
			c.Next()
			return
		}

		gormDB, ok := db.(*gorm.DB)
		if !ok {
			c.Next()
			return
		}

		// Execute subsequent processing first to ensure user information is set
		c.Next()

		// Get user information, skip logging if no user information
		userInfo := getUserInfo(c)
		if userInfo == nil {
			return
		}

		// Intelligently determine whether to log this operation based on configuration
		method := c.Request.Method
		path := c.Request.URL.Path
		if !operationLogConfig.ShouldLogOperation(method, path) {
			return
		}

		// Get request IP address
		ipAddress := c.ClientIP()

		// Get user agent information
		userAgent := c.GetHeader("User-Agent")

		// Get request referer
		referer := c.GetHeader("Referer")

		ua := user_agent.New(c.GetHeader("User-Agent"))
		device := ua.Platform()
		browser, version := ua.Browser()
		os := ua.OS()

		// Get geographic location information (based on IP)
		location := utils.GetRealAddressByIP(ipAddress)

		// Generate more detailed operation description
		action := c.Request.Method
		target := c.Request.URL.Path
		details := operationLogConfig.GetOperationDescription(action, target)

		// Record operation log (asynchronous execution to avoid affecting response time)
		go func() {
			err := CreateOperationLog(gormDB, userInfo.ID, userInfo.DisplayName, action, target, details, ipAddress, userAgent, referer, device, browser+version, os, location, action)
			if err != nil {
				// Log error but don't affect main flow
				log.Printf("Failed to record operation log: %v", err)
			}
		}()
	}
}

// getUserInfo extracts user information from context
func getUserInfo(c *gin.Context) *UserInfo {
	// Try to get user info from context
	user, exists := c.Get(constants.UserField)
	if !exists {
		return nil
	}

	// Try as UserInfo struct pointer first
	if info, ok := user.(*UserInfo); ok {
		return info
	}

	// Try as map
	if userMap, ok := user.(map[string]interface{}); ok {
		userID := extractUserID(userMap)
		displayName := extractDisplayName(userMap)
		if userID > 0 {
			return &UserInfo{
				ID:          userID,
				DisplayName: displayName,
			}
		}
	}

	// Try to get user_id and username separately from context
	userID := extractUserIDFromContext(c)
	displayName := extractDisplayNameFromContext(c)

	if userID > 0 {
		return &UserInfo{
			ID:          userID,
			DisplayName: displayName,
		}
	}

	return nil
}

// extractUserID extracts user ID from map with multiple key variations
func extractUserID(userMap map[string]interface{}) uint {
	for _, key := range []string{"id", "ID", "user_id", "UserID"} {
		if id, ok := userMap[key].(uint); ok {
			return id
		}
	}
	return 0
}

// extractDisplayName extracts display name from map with multiple key variations
func extractDisplayName(userMap map[string]interface{}) string {
	for _, key := range []string{"display_name", "DisplayName", "username", "Username"} {
		if name, ok := userMap[key].(string); ok {
			return name
		}
	}
	return ""
}

// extractUserIDFromContext extracts user ID from context with type conversion
func extractUserIDFromContext(c *gin.Context) uint {
	if uid, exists := c.Get("user_id"); exists {
		switch id := uid.(type) {
		case uint:
			return id
		case int:
			return uint(id)
		case int64:
			return uint(id)
		}
	}
	return 0
}

// extractDisplayNameFromContext extracts display name from context
func extractDisplayNameFromContext(c *gin.Context) string {
	for _, key := range []string{"username", "display_name"} {
		if name, exists := c.Get(key); exists {
			if n, ok := name.(string); ok {
				return n
			}
		}
	}
	return ""
}

// OperationLog represents user operation log
type OperationLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          uint      `gorm:"not null" json:"user_id"`          // User ID who performed the operation
	Username        string    `gorm:"not null" json:"username"`         // Username who performed the operation
	Action          string    `gorm:"not null" json:"action"`           // Operation type (e.g., create, delete, update)
	Target          string    `gorm:"not null" json:"target"`           // Operation target (e.g., user, order)
	Details         string    `gorm:"not null" json:"details"`          // Operation detailed description
	IPAddress       string    `gorm:"not null" json:"ip_address"`       // User IP address
	UserAgent       string    `gorm:"not null" json:"user_agent"`       // User browser information
	Referer         string    `gorm:"not null" json:"referer"`          // Request referer page
	Device          string    `gorm:"not null" json:"device"`           // User device (mobile, desktop, etc.)
	Browser         string    `gorm:"not null" json:"browser"`          // Browser information (e.g., Chrome, Firefox)
	OperatingSystem string    `gorm:"not null" json:"operating_system"` // Operating system (e.g., Windows, MacOS)
	Location        string    `gorm:"not null" json:"location"`         // User geographic location
	RequestMethod   string    `gorm:"not null" json:"request_method"`   // HTTP request method (GET, POST, etc.)
	CreatedAt       time.Time `json:"created_at"`                       // Operation time
}

// TableName specifies table name
func (OperationLog) TableName() string {
	return "operation_logs"
}

// CreateOperationLog creates an operation log
func CreateOperationLog(db *gorm.DB, userID uint, username, action, target, details, ipAddress, userAgent, referer, device, browser, operatingSystem, location, requestMethod string) error {
	log := OperationLog{
		UserID:          userID,
		Username:        username,
		Action:          action,
		Target:          target,
		Details:         details,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
		Referer:         referer,
		Device:          device,
		Browser:         browser,
		OperatingSystem: operatingSystem,
		Location:        location,
		RequestMethod:   requestMethod,
		CreatedAt:       time.Now(),
	}

	// Save operation log to database
	if err := db.Create(&log).Error; err != nil {
		return err
	}
	return nil
}
