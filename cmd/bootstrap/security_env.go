package bootstrap

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ValidateProductionSecurityEnv refuses to start in release/production when
// known fail-open toggles are enabled.
func ValidateProductionSecurityEnv() {
	if !productionLikeRuntime() {
		return
	}
	checks := []struct {
		env  string
		desc string
	}{
		{constants.ENVUploadsRecordingsPublic, "public SIP recordings under /uploads"},
		{constants.ENVWebSeatAllowEmptyToken, "WebSeat WS without token"},
		{constants.ENVVoiceDialogAllowEmptyToken, "VoiceDialog WS without token"},
		{constants.ENVTenantSelfRegister, "public tenant self-registration"},
	}
	for _, c := range checks {
		if utils.GetBoolEnv(c.env) {
			logger.Fatal("unsafe env for production",
				zap.String("env", c.env),
				zap.String("reason", c.desc),
			)
		}
	}
}

func productionLikeRuntime() bool {
	if gin.Mode() == gin.ReleaseMode {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(utils.GetEnv("APP_ENV"))) {
	case "production", "prod":
		return true
	default:
		return false
	}
}
