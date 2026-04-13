package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 生成 HMAC 签名
func generateSignature(data, secretKey string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// API 签名验证中间件
func SignVerifyMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中获取 API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.GetHeader("API-Key")
			if apiKey == "" {
				response.FailWithCode(c, 401, "API Key is missing", nil)
				c.Abort()
				return
			}
		}

		// 从请求头中获取 API Secret
		apiSecret := c.GetHeader("X-API-Secret")
		if apiSecret == "" {
			apiSecret = c.GetHeader("API-Secret")
			if apiSecret == "" {
				response.FailWithCode(c, 401, "API Secret is missing", nil)
				c.Abort()
				return
			}
		}

		// 从数据库中查找凭证
		credential, err := models.GetUserCredentialByApiSecretAndApiKey(db, apiKey, apiSecret)
		if err != nil {
			response.Fail(c, "Database error", nil)
			c.Abort()
			return
		}

		if credential == nil {
			response.FailWithCode(c, 401, "Invalid API credentials", nil)
			c.Abort()
			return
		}

		// 检查凭证状态
		if credential.IsBanned() {
			response.FailWithCode(c, 403, "Credential is banned", gin.H{
				"reason":   credential.BannedReason,
				"bannedAt": credential.BannedAt,
			})
			c.Abort()
			return
		}

		if credential.IsExpired() {
			response.FailWithCode(c, 403, "Credential has expired", gin.H{
				"expiresAt": credential.ExpiresAt,
			})
			c.Abort()
			return
		}

		if !credential.IsActive() {
			response.FailWithCode(c, 403, "Credential is not active", gin.H{
				"status": credential.Status,
			})
			c.Abort()
			return
		}
		c.Set("credential_id", credential.ID)
		c.Set("user_id", credential.UserID)

		// 签名验证通过，继续处理请求
		c.Next()
	}
}

// abs 返回绝对值
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// GetCredentialIDFromContext 从 Gin 上下文中获取凭证ID
func GetCredentialIDFromContext(c *gin.Context) (uint, bool) {
	credentialID, exists := c.Get("credential_id")
	if !exists {
		return 0, false
	}

	id, ok := credentialID.(uint)
	return id, ok
}

// GetUserIDFromContext 从 Gin 上下文中获取用户ID
func GetUserIDFromContext(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	id, ok := userID.(uint)
	return id, ok
}

// GetCredentialFromContext 从 Gin 上下文中获取完整凭证信息（需要数据库查询）
func GetCredentialFromContext(c *gin.Context, db *gorm.DB) (*models.Credential, error) {
	credentialID, exists := GetCredentialIDFromContext(c)
	if !exists {
		return nil, fmt.Errorf("credential_id not found in context")
	}

	var credential models.Credential
	if err := db.First(&credential, credentialID).Error; err != nil {
		return nil, err
	}

	return &credential, nil
}

// RequireCredential 中间件：确保请求包含有效的凭证（通常与其他中间件组合使用）
func RequireCredential() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := GetCredentialIDFromContext(c)
		if !exists {
			response.FailWithCode(c, 401, "Valid credential required", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
