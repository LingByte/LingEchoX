package constants

// Outbound campaign and contact queue statuses.

const (
	SIPCampaignStatusDraft       = "draft"
	SIPCampaignStatusRunning     = "running"
	SIPCampaignStatusPaused      = "paused"
	SIPCampaignStatusDone        = "done"
	SIPCampaignContactReady      = "ready"
	SIPCampaignContactDialing    = "dialing"
	SIPCampaignContactAnswered   = "answered"
	SIPCampaignContactFailed     = "failed"
	SIPCampaignContactRetrying   = "retrying"
	SIPCampaignContactExhausted  = "exhausted"
	SIPCampaignContactSuppressed = "suppressed"
)
