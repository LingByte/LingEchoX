package models

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/constants"
	"gorm.io/gorm"
)

const (
	CredentialStatusActive   = "active"
	CredentialStatusDisabled = "disabled"
)

// Credential is tenant-scoped API access (AK/SK) with optional IP allowlist.
type Credential struct {
	BaseModel

	TenantID  uint   `json:"tenantId" gorm:"index;not null"`
	Name      string `json:"name" gorm:"size:128"`
	AccessKey string `json:"accessKey" gorm:"size:64;uniqueIndex:idx_credential_ak;not null"`
	SecretKey string `json:"-" gorm:"size:256;not null"`
	Status    string `json:"status" gorm:"size:24;index;not null;default:active"` // active | disabled
	AllowIP   string `json:"allowIp,omitempty" gorm:"type:text;comment:白名单IP，多个逗号分隔"`
	// PermissionCodes JSON array of catalog codes (e.g. ["api.sip.calls.read"]); ["*"] = all; empty legacy = treat as *.
	PermissionCodes string `json:"permissionCodes,omitempty" gorm:"column:permission_codes;type:text"`
}

func (Credential) TableName() string {
	return constants.CREDENTIAL_TABLE_NAME
}

// CredentialMatchesPermissionCodes checks AK/SK permission JSON against required route codes (requireAll = AND).
func CredentialMatchesPermissionCodes(db *gorm.DB, credID uint, required []string, requireAll bool) (bool, error) {
	var row Credential
	if err := db.Where("id = ?", credID).First(&row).Error; err != nil {
		return false, err
	}
	raw := strings.TrimSpace(row.PermissionCodes)
	var codes []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &codes); err != nil {
			return false, err
		}
	} else {
		codes = []string{"*"}
	}
	for _, c := range codes {
		if strings.TrimSpace(c) == "*" {
			return true, nil
		}
	}
	if len(required) == 0 {
		return true, nil
	}
	if requireAll {
		for _, req := range required {
			if !slices.Contains(codes, req) {
				return false, nil
			}
		}
		return true, nil
	}
	for _, req := range required {
		if slices.Contains(codes, req) {
			return true, nil
		}
	}
	return false, nil
}

func GetActiveCredentialByAccessKey(db *gorm.DB, ak string) (Credential, error) {
	var row Credential
	err := db.Model(&Credential{}).
		Where("access_key = ? AND status = ?", ak, CredentialStatusActive).
		First(&row).Error
	return row, err
}

// CredentialClientIPAllowed reports whether clientIP is permitted by AllowIP (comma-separated).
// Empty or whitespace-only AllowIP means allow all.
func CredentialClientIPAllowed(allowList, clientIP string) bool {
	allowList = strings.TrimSpace(allowList)
	if allowList == "" {
		return true
	}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return false
	}
	for _, part := range strings.Split(allowList, ",") {
		if strings.TrimSpace(part) == clientIP {
			return true
		}
	}
	return false
}

// CredentialSignPathWithSortedQuery returns path + "?" + url.Values.Encode() for stable query ordering.
func CredentialSignPathWithSortedQuery(path string, rawQuery string) string {
	path = strings.TrimSpace(path)
	if rawQuery == "" {
		return path
	}
	v, err := url.ParseQuery(rawQuery)
	if err != nil {
		return path + "?" + rawQuery
	}
	q := v.Encode()
	if q == "" {
		return path
	}
	return path + "?" + q
}

// CredentialSignSHA256Hex is lowercase hex SHA-256 of body.
func CredentialSignSHA256Hex(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// CredentialBuildStringToSign builds canonical payload for AK/SK HMAC:
//
//	METHOD\npathWithSortedQuery\nUNIX_TS\nSHA256Hex(body)
func CredentialBuildStringToSign(methodUpper, pathWithSortedQuery, ts string, body []byte) string {
	var b strings.Builder
	b.WriteString(methodUpper)
	b.WriteByte('\n')
	b.WriteString(pathWithSortedQuery)
	b.WriteByte('\n')
	b.WriteString(ts)
	b.WriteByte('\n')
	b.WriteString(CredentialSignSHA256Hex(body))
	return b.String()
}

// CredentialSignHex returns lowercase hex HMAC-SHA256(secretKey, message).
func CredentialSignHex(secretKey, message string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
