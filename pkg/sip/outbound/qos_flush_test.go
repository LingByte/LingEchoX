// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import "testing"

// flushOutboundCallQoS must be safe against the two nil branches it
// hits in practice: a nil leg (defensive) and a leg whose rtpSess
// was never set (e.g. failures before SDP answer arrived).
func TestFlushOutboundCallQoS_NilSafe(t *testing.T) {
	// Should not panic, should not panic, should not panic.
	flushOutboundCallQoS(nil)
	flushOutboundCallQoS(&outLeg{})
}
