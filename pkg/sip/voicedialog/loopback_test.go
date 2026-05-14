package voicedialog

import "testing"

func TestStartInboundLoopbackWS_NoOp(t *testing.T) {
	var h *Hub
	h.startInboundLoopbackWS(nil)

	h = &Hub{cfg: Config{}}
	h.startInboundLoopbackWS(nil)

	sess := &dialogSession{meta: InboundMeta{CallID: "x"}}
	h.startInboundLoopbackWS(sess)

	h.cfg.InboundLoopbackWS = true
	h.cfg.LoopbackHTTPHostPort = ""
	h.startInboundLoopbackWS(sess)
}
