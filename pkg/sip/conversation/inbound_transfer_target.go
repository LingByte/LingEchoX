package conversation

import "strings"

// TakeInboundTransferACDTargetID returns the last selected acd_pool_targets.id for this inbound Call-ID and clears it.
// It is best-effort; returns 0 when unknown.
func TakeInboundTransferACDTargetID(callID string) uint {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return 0
	}
	if v, ok := transferLastACDRowByInbound.LoadAndDelete(callID); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}
