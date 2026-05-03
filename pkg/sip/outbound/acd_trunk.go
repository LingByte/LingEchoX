package outbound

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
)

// DialTargetFromACDTrunk builds INVITE target from ACD pool trunk row (user part + gateway).
// userPart is TargetValue (digits or dial string); host is SipTrunkHost; port defaults to 6050 when invalid.
// If signalingOverride is empty, SignalingAddr is host:port.
func DialTargetFromACDTrunk(userPart, host, signalingOverride string, port int) (DialTarget, bool) {
	userPart = strings.TrimSpace(userPart)
	host = strings.TrimSpace(host)
	if userPart == "" {
		return DialTarget{}, false
	}
	if host == "" {
		host = strings.TrimSpace(utils.GetEnv(constants.EnvSIPTransferHost))
	}
	if host == "" {
		host = strings.TrimSpace(utils.GetEnv(constants.EnvSIPOutboundHost))
	}
	if host == "" {
		return DialTarget{}, false
	}
	if port <= 0 || port >= 65536 {
		port = 50400
		if ps := strings.TrimSpace(utils.GetEnv(constants.EnvSIPTransferPort)); ps != "" {
			if p, err := strconv.Atoi(ps); err == nil && p > 0 && p < 65536 {
				port = p
				logger.Info("parse ture", zap.Int("port", port))
			} else {
				logger.Error("parse error", zap.Error(err))
			}
		}
	}

	var t DialTarget
	t.RequestURI = normalizeSIPRequestURI(fmt.Sprintf("sip:%s@%s:%d", userPart, host, port))
	sig := strings.TrimSpace(signalingOverride)
	if sig == "" {
		sig = strings.TrimSpace(utils.GetEnv(constants.EnvSIPTransferSigAddr))
	}
	if sig != "" {
		t.SignalingAddr = sig
	} else {
		t.SignalingAddr = fmt.Sprintf("%s:%d", host, port)
	}
	return t, true
}
