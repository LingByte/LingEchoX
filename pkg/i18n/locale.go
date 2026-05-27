package i18n

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	LocaleZhCN = "zh-CN"
	LocaleEnUS = "en-US"
)

const ctxLocaleKey = "i18n.locale"

// NormalizeLocale maps Accept-Language / query values to supported locales.
func NormalizeLocale(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return LocaleZhCN
	}
	if strings.HasPrefix(raw, "en") {
		return LocaleEnUS
	}
	if strings.HasPrefix(raw, "zh") {
		return LocaleZhCN
	}
	switch raw {
	case LocaleEnUS, "en_us", "en":
		return LocaleEnUS
	case LocaleZhCN, "zh_cn", "zh":
		return LocaleZhCN
	default:
		return LocaleZhCN
	}
}

// LocaleFromGin reads locale from context (set by LocaleMiddleware).
func LocaleFromGin(c *gin.Context) string {
	if c == nil {
		return LocaleZhCN
	}
	if v, ok := c.Get(ctxLocaleKey); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return LocaleZhCN
}

// SetLocaleOnGin stores locale on gin context.
func SetLocaleOnGin(c *gin.Context, locale string) {
	if c != nil {
		c.Set(ctxLocaleKey, NormalizeLocale(locale))
	}
}

// ParseAcceptLanguage picks the best supported locale from Accept-Language.
func ParseAcceptLanguage(header string) string {
	if header == "" {
		return LocaleZhCN
	}
	parts := strings.Split(header, ",")
	for _, part := range parts {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if tag == "" {
			continue
		}
		loc := NormalizeLocale(tag)
		if loc == LocaleEnUS || loc == LocaleZhCN {
			return loc
		}
	}
	return LocaleZhCN
}
