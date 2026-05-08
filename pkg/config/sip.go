package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
)

// SIPDialEnv holds SIP dial fields parsed from SIP_TRANSFER_* environment variables.
// Callers map this to pkg/sip/outbound.DialTarget at the SIP boundary so this package does not import outbound
// (outbound HTTP helpers live in the same module and would create an import cycle).
type SIPDialEnv struct {
	RequestURI    string
	SignalingAddr string
	WebSeat       bool
}

// CallerIdentityFromEnv reads SIP_CALLER_ID / SIP_CALLER_DISPLAY_NAME for outbound INVITE From/Contact.
// User is the SIP URI user part; displayName is optional (empty → From has no quoted display-name).
func CallerIdentityFromEnv() (user, displayName string) {
	user = utils.GetEnv(constants.EnvSIPCallerID)
	displayName = utils.GetEnv(constants.EnvSIPCallerDisplayName)
	return user, displayName
}

// RegisterPasswordFromEnv returns SIP_PASSWORD when set (trimmed). Empty means REGISTER is open
// (no shared secret). Non-empty means clients must send matching X-SIP-Register-Password on REGISTER.
func RegisterPasswordFromEnv() string {
	return utils.GetEnv(constants.EnvSIPRegisterPassword)
}

// TransferDialTargetFromEnv reads SIP_TRANSFER_* (agent extension for blind transfer dial).
func TransferDialTargetFromEnv() (t SIPDialEnv, ok bool) {
	sig := utils.GetEnv(constants.EnvSIPTransferSigAddr)
	num := utils.GetEnv(constants.EnvSIPTransferNumber)
	if strings.EqualFold(num, "web") {
		return SIPDialEnv{WebSeat: true}, true
	}
	host := utils.GetEnv(constants.EnvSIPTransferHost)
	if num == "" || host == "" {
		return SIPDialEnv{}, false
	}

	port := 50400
	ps := utils.GetEnv(constants.EnvSIPTransferPort)
	if ps != "" {
		if p, err := strconv.Atoi(ps); err == nil && p > 0 && p < 65536 {
			port = p
			logger.Info("parse ture", zap.Int("port", port))
		} else {
			logger.Error("parse error", zap.Error(err))
		}
	}
	t.RequestURI = fmt.Sprintf("sip:%s@%s:%d", num, host, port)
	if sig == "" {
		t.SignalingAddr = fmt.Sprintf("%s:%d", host, port)
	} else {
		t.SignalingAddr = sig
	}
	return t, true
}

func MediaMaxSecondsFromEnv() int {
	const def = 512
	const minQ = 64
	const maxQ = 2048
	s := utils.GetEnv("SIP_MEDIA_MAX_SECONDS")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < minQ || n > maxQ {
		return def
	}
	return n
}

func MediaTxQueueSizeFromEnv() int {
	s := utils.GetEnv("SIP_MEDIA_TX_QUEUE_SIZE")
	if s == "" {
		return 3600
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 3600
	}
	return n
}
