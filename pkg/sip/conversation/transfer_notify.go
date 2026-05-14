package conversation

import (
	"strings"
	"sync"
)

// Transfer phase strings passed to SetTransferPhaseNotifier (voicedialog mirrors them as dialog.transfer.phase).
const (
	TransferPhaseConnected = "connected"
)

// TransferPhaseNotifier receives SIP transfer lifecycle phases for UI / voicedialog WebSocket events.
// phase examples: requested, loading, ringing, connected, failed, no_agent (nil-safe).
var transferPhaseMu sync.RWMutex
var transferPhaseNotifier func(callID string, phase string, fields map[string]any)

// SetTransferPhaseNotifier registers a callback (typically pkg/sip/voicedialog). Pass nil to clear.
func SetTransferPhaseNotifier(fn func(callID string, phase string, fields map[string]any)) {
	transferPhaseMu.Lock()
	defer transferPhaseMu.Unlock()
	transferPhaseNotifier = fn
}

func notifyTransferPhase(callID string, phase string, fields map[string]any) {
	callID = strings.TrimSpace(callID)
	phase = strings.TrimSpace(phase)
	if callID == "" || phase == "" {
		return
	}
	transferPhaseMu.RLock()
	fn := transferPhaseNotifier
	transferPhaseMu.RUnlock()
	if fn == nil {
		return
	}
	fn(callID, phase, fields)
}
