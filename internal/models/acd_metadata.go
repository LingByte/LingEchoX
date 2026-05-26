package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxACDMetaDataBytes caps acd_pool_targets.meta_data JSON blob size.
const MaxACDMetaDataBytes = 4096

// MaxACDRemarkLen matches BaseModel.Remark column size.
const MaxACDRemarkLen = 128

// NormalizeACDRemark trims plain-text admin remark (not the JSON meta blob).
func NormalizeACDRemark(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if len([]rune(s)) > MaxACDRemarkLen {
		return "", fmt.Errorf("remark exceeds %d characters", MaxACDRemarkLen)
	}
	return s, nil
}

// NormalizeACDMetaDataJSON accepts empty, JSON object string, or json.RawMessage / map from API.
func NormalizeACDMetaDataJSON(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	switch v := raw.(type) {
	case string:
		return normalizeACDMetaDataString(v)
	case []byte:
		return normalizeACDMetaDataString(string(v))
	case json.RawMessage:
		return normalizeACDMetaDataString(string(v))
	case map[string]any:
		return marshalACDMetaMap(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("metaData must be a JSON object")
		}
		return normalizeACDMetaDataString(string(b))
	}
}

func normalizeACDMetaDataString(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" || s == "null" {
		return "", nil
	}
	if len(s) > MaxACDMetaDataBytes {
		return "", fmt.Errorf("metaData exceeds %d bytes", MaxACDMetaDataBytes)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return "", fmt.Errorf("metaData must be valid JSON object: %w", err)
	}
	return marshalACDMetaMap(m)
}

func marshalACDMetaMap(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("metaData marshal failed: %w", err)
	}
	if len(b) > MaxACDMetaDataBytes {
		return "", fmt.Errorf("metaData exceeds %d bytes", MaxACDMetaDataBytes)
	}
	return string(b), nil
}

// ParseACDMetaDataMap decodes row.MetaData; invalid/empty → empty map.
func ParseACDMetaDataMap(metaData string) map[string]any {
	metaData = strings.TrimSpace(metaData)
	if metaData == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(metaData), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// LookupACDMetaPath resolves dotted paths in metadata, e.g. FactoryNumber or nested a.b.c.
func LookupACDMetaPath(meta map[string]any, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(meta) == 0 {
		return ""
	}
	parts := strings.Split(path, ".")
	var cur any = meta
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		switch node := cur.(type) {
		case map[string]any:
			cur = node[p]
		default:
			return ""
		}
	}
	return metaValueToString(cur)
}

func metaValueToString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "是"
		}
		return "否"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
}
