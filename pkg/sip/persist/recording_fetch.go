package persist

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/stores"
)

// FetchRecordingWAV loads recording bytes from recording_url (http(s), storage key, or local uploads path).
func FetchRecordingWAV(recordingURL string, httpClient *http.Client) ([]byte, error) {
	raw := strings.TrimSpace(recordingURL)
	if raw == "" {
		return nil, fmt.Errorf("empty recording url")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	if key := recordingStorageKey(raw); key != "" {
		if b, err := readFromStore(key); err == nil && len(b) > 0 {
			return b, nil
		}
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return fetchHTTP(httpClient, raw)
	}

	if key := recordingStorageKey(raw); key != "" {
		return readFromStore(key)
	}
	return nil, fmt.Errorf("unsupported recording url: %s", raw)
}

func fetchHTTP(client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s: %s", resp.Status, u)
	}
	const maxWAV = 128 << 20 // 128 MiB safety cap
	return io.ReadAll(io.LimitReader(resp.Body, maxWAV))
}

func readFromStore(key string) ([]byte, error) {
	st := stores.Default()
	if st == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	rc, _, err := st.Read(key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	const maxWAV = 128 << 20
	return io.ReadAll(io.LimitReader(rc, maxWAV))
}

// recordingStorageKey extracts sip/recordings/... object key from a URL or uploads path.
func recordingStorageKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		p := strings.TrimPrefix(strings.TrimSpace(u.Path), "/")
		if idx := strings.Index(p, "sip/recordings/"); idx >= 0 {
			return p[idx:]
		}
		return p
	}
	p := strings.TrimPrefix(raw, "/")
	if idx := strings.Index(p, "sip/recordings/"); idx >= 0 {
		return p[idx:]
	}
	return p
}
