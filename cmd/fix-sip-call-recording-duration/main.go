// fix-sip-call-recording-duration reads each sip_calls.recording_url WAV, computes real
// audio duration from the file header, and updates duration_sec / ended_at / bye_at.
//
// Usage (from repo root, with .env / DSN configured):
//
//	go run ./cmd/fix-sip-call-recording-duration -dry-run -limit 20
//	go run ./cmd/fix-sip-call-recording-duration -dry-run=false -ids 14422,14425
//	go run ./cmd/fix-sip-call-recording-duration -dry-run=false -all
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/config"
	sipPersist "github.com/LinByte/VoiceServer/pkg/sip/persist"
	"github.com/LinByte/VoiceServer/pkg/utils"
)

func main() {
	dryRun := flag.Bool("dry-run", true, "print changes only; pass -dry-run=false to write DB")
	all := flag.Bool("all", false, "process all ended calls with recording_url (default: only suspicious duration/timestamps)")
	idsFlag := flag.String("ids", "", "comma-separated sip_calls.id list")
	limit := flag.Int("limit", 0, "max rows to process (0 = no limit)")
	httpTimeout := flag.Duration("http-timeout", 90*time.Second, "timeout fetching remote recording URLs")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := utils.InitDatabase(os.Stdout, config.GlobalConfig.Database.Driver, config.GlobalConfig.Database.DSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	idFilter, err := parseIDs(*idsFlag)
	if err != nil {
		log.Fatalf("ids: %v", err)
	}

	var rows []sipPersist.SIPCall
	q := sipPersist.ActiveSIPCalls(db).
		Where("state = ?", sipPersist.SIPCallStateEnded).
		Where("recording_url <> ?", "").
		Order("id DESC")
	if len(idFilter) > 0 {
		q = q.Where("id IN ?", idFilter)
	}
	if *limit > 0 {
		q = q.Limit(*limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Fatalf("query: %v", err)
	}

	httpClient := &http.Client{Timeout: *httpTimeout}
	var scanned, updated, skipped, failed int

	for _, row := range rows {
		scanned++
		if !*all && len(idFilter) == 0 && !needsFix(&row) {
			skipped++
			continue
		}

		wav, err := sipPersist.FetchRecordingWAV(row.RecordingURL, httpClient)
		if err != nil {
			failed++
			log.Printf("id=%d call_id=%s fetch: %v", row.ID, row.CallID, err)
			continue
		}
		durSec := sipPersist.WAVDurationSec(wav)
		if durSec <= 0 {
			failed++
			log.Printf("id=%d call_id=%s parse wav duration failed (bytes=%d)", row.ID, row.CallID, len(wav))
			continue
		}

		start := callStartTime(&row)
		if start.IsZero() {
			failed++
			log.Printf("id=%d call_id=%s missing ack_at/invite_at", row.ID, row.CallID)
			continue
		}
		newEnd := start.Add(time.Duration(durSec) * time.Second)
		oldDur := row.DurationSec
		if oldDur <= 0 {
			oldDur = sipPersist.ComputeCallDurationSec(&row)
		}

		log.Printf("id=%d call_id=%s wav_bytes=%d duration_sec %d -> %d ended_at %s -> %s",
			row.ID, row.CallID, len(wav), oldDur, durSec,
			fmtTime(row.EndedAt), newEnd.Format(time.RFC3339))

		if *dryRun {
			continue
		}

		upd := map[string]any{
			"duration_sec":        durSec,
			"recording_wav_bytes": len(wav),
			"ended_at":            newEnd,
			"bye_at":              newEnd,
			"updated_at":          time.Now(),
		}
		if err := db.Model(&sipPersist.SIPCall{}).Where("id = ?", row.ID).Updates(upd).Error; err != nil {
			failed++
			log.Printf("id=%d update: %v", row.ID, err)
			continue
		}
		updated++
	}

	log.Printf("done scanned=%d updated=%d skipped=%d failed=%d dry_run=%v", scanned, updated, skipped, failed, *dryRun)
}

func needsFix(c *sipPersist.SIPCall) bool {
	if c == nil {
		return false
	}
	wall := sipPersist.ComputeCallDurationSec(c)
	if c.DurationSec > 0 && wall > 0 {
		diff := c.DurationSec - wall
		if diff < 0 {
			diff = -diff
		}
		if diff <= 5 {
			return false
		}
	}
	if c.DurationSec > 3600 {
		return true
	}
	if c.EndedAt != nil && c.ByeAt != nil && c.EndedAt.Sub(*c.ByeAt).Abs() > 2*time.Minute {
		return true
	}
	if c.EndedAt != nil && c.AckAt != nil {
		endFromAck := int(c.EndedAt.Sub(*c.AckAt).Seconds())
		if c.DurationSec > 0 && endFromAck > c.DurationSec+120 {
			return true
		}
	}
	return c.DurationSec <= 0
}

func callStartTime(c *sipPersist.SIPCall) time.Time {
	if c == nil {
		return time.Time{}
	}
	if c.AckAt != nil && !c.AckAt.IsZero() {
		return *c.AckAt
	}
	if c.InviteAt != nil && !c.InviteAt.IsZero() {
		return *c.InviteAt
	}
	return time.Time{}
}

func parseIDs(s string) ([]uint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q", p)
		}
		out = append(out, uint(n))
	}
	return out, nil
}

func fmtTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.Format(time.RFC3339)
}
