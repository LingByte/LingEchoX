// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
)

func TestApplyRetargetHeaders_FreshCallChain(t *testing.T) {
	// Inbound INVITE had no pre-existing chain. We should emit a 2-entry
	// History-Info (original-To, agent) + 1-entry Diversion.
	req := outbound.DialRequest{
		Target: outbound.DialTarget{RequestURI: "sip:agent42@pool.example"},
	}
	applyRetargetHeaders(&req,
		"<sip:+8613800138000@trunk.example>;tag=abc",
		"", "",
		`SIP;cause=302;text="Transfer"`,
		historyinfo.DiversionUnconditional,
	)
	if len(req.HistoryInfo) != 2 {
		t.Fatalf("HistoryInfo entries = %d, want 2", len(req.HistoryInfo))
	}
	if req.HistoryInfo[0].URI != "sip:+8613800138000@trunk.example" || req.HistoryInfo[0].Index != "1" {
		t.Errorf("root entry = %+v", req.HistoryInfo[0])
	}
	if req.HistoryInfo[1].URI != "sip:agent42@pool.example" || req.HistoryInfo[1].Index != "2" {
		t.Errorf("agent entry = %+v", req.HistoryInfo[1])
	}
	if got := req.HistoryInfo[1].ReasonHeader; !strings.Contains(got, "cause=302") {
		t.Errorf("agent entry reason = %q", got)
	}
	if len(req.Diversion) != 1 || req.Diversion[0].URI != "sip:+8613800138000@trunk.example" ||
		req.Diversion[0].Counter != 1 || req.Diversion[0].Reason != historyinfo.DiversionUnconditional {
		t.Errorf("Diversion = %+v", req.Diversion)
	}
}

func TestApplyRetargetHeaders_ExtendsUpstreamChain(t *testing.T) {
	// Inbound had two upstream History-Info entries from carrier SBC.
	// We must extend, not replace, and our new entry's index must be 3.
	upstreamHI := `<sip:carrier-sbc@carrier.net>;index=1, <sip:+8613800138000@trunk.example>;index=2`
	upstreamDV := `<sip:carrier-sbc@carrier.net>;reason=unconditional;counter=1`
	req := outbound.DialRequest{
		Target: outbound.DialTarget{RequestURI: "sip:agent42@pool.example"},
	}
	applyRetargetHeaders(&req,
		"<sip:+8613800138000@trunk.example>",
		upstreamHI, upstreamDV,
		`SIP;cause=302`,
		historyinfo.DiversionDeflection,
	)
	if len(req.HistoryInfo) != 3 {
		t.Fatalf("HistoryInfo entries = %d, want 3 (chain must extend)", len(req.HistoryInfo))
	}
	if req.HistoryInfo[2].Index != "3" {
		t.Errorf("new entry index = %q, want 3", req.HistoryInfo[2].Index)
	}
	if len(req.Diversion) != 2 || req.Diversion[1].Counter != 2 {
		t.Errorf("Diversion chain failed to extend: %+v", req.Diversion)
	}
}

func TestApplyRetargetHeaders_NoInboundHistorySkips(t *testing.T) {
	// If we have nothing to anchor on (no To, no upstream chain),
	// emitting a chain with only the agent entry would mislead
	// downstream into thinking a redirect happened. Should leave req
	// untouched.
	req := outbound.DialRequest{
		Target: outbound.DialTarget{RequestURI: "sip:agent42@pool.example"},
	}
	applyRetargetHeaders(&req, "", "", "", "SIP;cause=302", "unconditional")
	if req.HistoryInfo != nil || req.Diversion != nil {
		t.Errorf("expected no-op, got HI=%+v DV=%+v", req.HistoryInfo, req.Diversion)
	}
}

func TestApplyRetargetHeaders_EmptyTargetIsNoop(t *testing.T) {
	req := outbound.DialRequest{}
	applyRetargetHeaders(&req,
		"<sip:+8613800138000@trunk.example>",
		"", "",
		"SIP;cause=302",
		"unconditional",
	)
	if req.HistoryInfo != nil || req.Diversion != nil {
		t.Errorf("empty target should produce no chain; got HI=%+v DV=%+v", req.HistoryInfo, req.Diversion)
	}
}

func TestApplyRetargetHeaders_NilRequestSafe(t *testing.T) {
	// Must not panic.
	applyRetargetHeaders(nil, "", "", "", "", "")
}
