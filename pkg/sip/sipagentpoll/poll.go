// Package sipagentpoll tracks in-flight SIP-seat transfer rings for simple HTTP polling.
package sipagentpoll

import (
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/internal/models"
)

// Snapshot is returned by GET …/sip-agent/incoming (one row per acd_pool_targets.id).
type Snapshot struct {
	Incoming      bool   `json:"incoming"`
	InboundCallID string `json:"callId,omitempty"`
	CallerNumber  string `json:"callerNumber,omitempty"`
	Phase         string `json:"phase,omitempty"` // ringing
	ACDTargetID   uint   `json:"acdTargetId,omitempty"`
	Since         string `json:"since,omitempty"` // RFC3339
}

type entry struct {
	inbound      string
	callerNumber string
	phase        string
	since        time.Time
}

var (
	byACD        sync.Map // uint -> *entry
	inboundToACD sync.Map // inbound Call-ID -> uint
)

// SetSIPAgentRinging marks that an inbound call is being transferred to this SIP ACD row.
func SetSIPAgentRinging(acdTargetID uint, inboundCallID, callerNumber string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if acdTargetID == 0 || inboundCallID == "" {
		return
	}
	callerNumber = strings.TrimSpace(callerNumber)
	if old, ok := inboundToACD.Load(inboundCallID); ok {
		if prev, _ := old.(uint); prev != 0 && prev != acdTargetID {
			byACD.Delete(prev)
		}
	}
	inboundToACD.Store(inboundCallID, acdTargetID)
	byACD.Store(acdTargetID, &entry{
		inbound:      inboundCallID,
		callerNumber: callerNumber,
		phase:        models.SIPACDTransferOfferPhaseRinging,
		since:        time.Now().UTC(),
	})
	persistStartRinging(acdTargetID, inboundCallID, callerNumber)
	NotifyACDTargetChanged(acdTargetID)
}

func clearMemoryByInbound(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	v, ok := inboundToACD.LoadAndDelete(inboundCallID)
	if !ok || v == nil {
		return
	}
	if id, ok := v.(uint); ok && id != 0 {
		byACD.Delete(id)
		NotifyACDTargetChanged(id)
	}
}

// ClearByInbound removes in-memory ringing state and marks DB rows cancelled (hangup / cleanup).
func ClearByInbound(inboundCallID string) {
	clearMemoryByInbound(inboundCallID)
	persistFinishInbound(inboundCallID, models.SIPACDTransferOfferPhaseCancelled)
}

// ClearByACDTarget clears in-memory state for one pool row and marks DB rows superseded.
func ClearByACDTarget(acdTargetID uint) {
	if acdTargetID == 0 {
		return
	}
	v, ok := byACD.LoadAndDelete(acdTargetID)
	if !ok || v == nil {
		persistFinishACD(acdTargetID, models.SIPACDTransferOfferPhaseSuperseded)
		return
	}
	if e, ok := v.(*entry); ok && e != nil {
		inboundToACD.Delete(strings.TrimSpace(e.inbound))
	}
	persistFinishACD(acdTargetID, models.SIPACDTransferOfferPhaseSuperseded)
	NotifyACDTargetChanged(acdTargetID)
}

// MarkInboundConnected ends ringing offers when the agent leg is answered / bridged.
func MarkInboundConnected(inboundCallID string) {
	clearMemoryByInbound(inboundCallID)
	persistFinishInbound(inboundCallID, models.SIPACDTransferOfferPhaseConnected)
}

// MarkInboundFailed ends ringing offers when the agent leg fails before connect.
func MarkInboundFailed(inboundCallID string) {
	clearMemoryByInbound(inboundCallID)
	persistFinishInbound(inboundCallID, models.SIPACDTransferOfferPhaseFailed)
}

// SnapshotByACDTarget returns whether this SIP seat currently has a ringing transfer.
func SnapshotByACDTarget(acdTargetID uint) Snapshot {
	if acdTargetID == 0 {
		return Snapshot{Incoming: false}
	}
	v, ok := byACD.Load(acdTargetID)
	if !ok || v == nil {
		return Snapshot{Incoming: false, ACDTargetID: acdTargetID}
	}
	e, ok := v.(*entry)
	if !ok || e == nil {
		return Snapshot{Incoming: false, ACDTargetID: acdTargetID}
	}
	return Snapshot{
		Incoming:      true,
		InboundCallID: e.inbound,
		CallerNumber:  e.callerNumber,
		Phase:         e.phase,
		ACDTargetID:   acdTargetID,
		Since:         e.since.Format(time.RFC3339),
	}
}

// maxDBRingingAge is the upper bound we trust a DB-only ringing row without in-memory state.
const maxDBRingingAge = 3 * time.Minute

// ResolveSnapshot returns poll state for one ACD seat. In-memory wins; DB is fallback only
// while the inbound leg is still in a live transfer/ring window (isTransferLive) and the row
// is not older than maxDBRingingAge. Stale DB rows are closed as cancelled so HTTP poll clears
// promptly after PSTN/agent hangup even if a cleanup hook was missed.
func ResolveSnapshot(acdTargetID uint, isTransferLive func(inboundCallID string) bool) Snapshot {
	if acdTargetID == 0 {
		return Snapshot{Incoming: false}
	}
	if snap := SnapshotByACDTarget(acdTargetID); snap.Incoming {
		inbound := strings.TrimSpace(snap.InboundCallID)
		if inbound != "" && isTransferLive != nil && !isTransferLive(inbound) {
			ClearByInbound(inbound)
			return Snapshot{Incoming: false, ACDTargetID: acdTargetID}
		}
		return snap
	}
	dbSnap, ok := activeOfferFromDBFresh(acdTargetID)
	if !ok || !dbSnap.Incoming {
		return Snapshot{Incoming: false, ACDTargetID: acdTargetID}
	}
	inbound := strings.TrimSpace(dbSnap.InboundCallID)
	if inbound != "" {
		if isTransferLive != nil && !isTransferLive(inbound) {
			ClearByInbound(inbound)
			return Snapshot{Incoming: false, ACDTargetID: acdTargetID}
		}
	}
	return dbSnap
}
