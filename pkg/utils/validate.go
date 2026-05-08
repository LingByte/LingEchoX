package utils

import (
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

var (
	mobileRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)
	slugRegex   = regexp.MustCompile(`^[a-z0-9\-_]{3,64}$`)
	domainRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-.]{1,61}[a-zA-Z0-9]\.[a-zA-Z]{2,}$`)
)

func IsEmail(email string) bool {
	_, err := mail.ParseAddress(strings.TrimSpace(email))
	return err == nil
}

func IsMobile(mobile string) bool {
	return mobileRegex.MatchString(mobile)
}

func IsDomain(domain string) bool {
	return domainRegex.MatchString(domain)
}

func IsSlug(slug string) bool {
	return slugRegex.MatchString(slug)
}

func IsEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}

func Trim(s string) string {
	return strings.TrimSpace(s)
}

func TrimAll(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "")
}

func TrimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func DefaultStr(s string, def string) string {
	if IsEmpty(s) {
		return def
	}
	return s
}

// NormalizePage clamps page/size into valid ranges.
// maxSize <= 0 defaults to 100.
func NormalizePage(page, size, maxSize int) (int, int) {
	if maxSize <= 0 {
		maxSize = 100
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > maxSize {
		size = maxSize
	}
	return page, size
}

// ParseID parses a string into a uint ID. Returns error if empty, non-numeric, or zero.
func ParseID(s string) (uint, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil || v == 0 {
		return 0, fmt.Errorf("invalid id: %q", s)
	}
	return uint(v), nil
}

// ValidPassword checks minimum password length.
func ValidPassword(pw string, minLen int) bool {
	if minLen <= 0 {
		minLen = 8
	}
	return len(strings.TrimSpace(pw)) >= minLen
}
