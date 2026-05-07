package sipserver

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	"github.com/LingByte/SoulNexus/pkg/sip/outbound"
	"github.com/LingByte/SoulNexus/pkg/sip/persist"
	"github.com/LingByte/SoulNexus/pkg/sip/server"
	"github.com/LingByte/SoulNexus/pkg/sip/stack"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	"github.com/LingByte/SoulNexus/pkg/sip/voicedialog"
	"github.com/LingByte/SoulNexus/pkg/sip/webseat"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Config controls the embedded SIP sidecars.
type Config struct {
	Host    string
	Port    int
	LocalIP string
	DB      *gorm.DB // Required: same pool as the HTTP app (REGISTER, sip_calls, campaign worker).
}

// Embedded holds started subsystems for graceful shutdown.
type Embedded struct {
	sipServer   *server.SIPServer
	campaignSvc *CampaignService
	outMgr      *outbound.Manager
}

func (e *Embedded) CampaignService() *CampaignService {
	if e == nil {
		return nil
	}
	return e.campaignSvc
}

// warnIfSIPViaLoopback detects the common misconfig where outbound INVITE puts Via/Contact on 127.0.0.1.
// Callees then send 100/180/200 to their own loopback — server never sees a response → timeout_no_final_response.
func warnIfSIPViaLoopback(sipHostEffective, localIP, sipListenHost string) {
	h := strings.TrimSpace(sipHostEffective)
	if h == "" {
		return
	}
	loopback := false
	switch strings.ToLower(h) {
	case "127.0.0.1", "::1", "localhost":
		loopback = true
	default:
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			loopback = true
		}
	}
	if !loopback {
		return
	}
	msg := "outbound/campaign INVITE Via uses a loopback address — phones on the LAN will send SIP responses to 127.0.0.1 on THEIR machine; " +
		"you will see INVITE dispatched then timeout_no_final_response with no ringing. " +
		"Fix: pass -sip-local-ip=<this server's LAN IP>, e.g. the same subnet as your SIP clients (listen host was %q, sip-local-ip=%q)."
	if logger.Lg != nil {
		logger.Lg.Warn("sipapp: SIP signaling advertise address is loopback",
			zap.String("sip_via_host_effective", h),
			zap.String("sip_listen_host", strings.TrimSpace(sipListenHost)),
			zap.String("sip_local_ip_flag", strings.TrimSpace(localIP)),
			zap.String("hint", fmt.Sprintf(msg, sipListenHost, localIP)),
		)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "sipapp WARN: %s\n", fmt.Sprintf(msg, sipListenHost, localIP))
	}
}

// resolveOutboundDialTarget 仅在启动时打一行「外呼试拨目标」日志使用。
// 优先级：
//  1. SIP_TARGET_NUMBER 命中已注册的 SIP user（store.DialTargetForUsername）；
//  2. tenantID=0 的平台级 Trunk + 一条允许外呼的 TrunkNumber + SIP_TARGET_NUMBER 作为被叫。
//
// 没有可用中继时直接返回 false；不再读 SIP_OUTBOUND_HOST 等 env 兜底。
func resolveOutboundDialTarget(db *gorm.DB, store *persist.GormStore) (outbound.DialTarget, bool) {
	if store != nil {
		if n := utils.GetEnv(constants.EnvSIPTargetNumber); n != "" {
			if dt, ok := store.DialTargetForUsername(context.Background(), n); ok {
				return dt, true
			}
		}
	}
	return buildOutboundDialTargetFromDB(db)
}

// buildOutboundDialTargetFromDB 用 PickTrunkOutboundConfig + SIP_TARGET_NUMBER 拼一个外呼目标。
//
// SIP_TARGET_NUMBER 仅作为「试拨被叫号码」用，不再作为主叫身份；主叫已由 TrunkNumber 提供。
// 找不到外呼 trunk 或 SIP_TARGET_NUMBER 为空均返回 false。
func buildOutboundDialTargetFromDB(db *gorm.DB) (outbound.DialTarget, bool) {
	if db == nil {
		return outbound.DialTarget{}, false
	}
	target := strings.TrimSpace(utils.GetEnv(constants.EnvSIPTargetNumber))
	if target == "" {
		return outbound.DialTarget{}, false
	}
	cfg, ok := models.PickTrunkOutboundConfig(db, 0)
	if !ok {
		return outbound.DialTarget{}, false
	}
	dt, ok := outbound.DialTargetFromACDTrunk(target, cfg.Host, "", cfg.Port)
	if !ok {
		return outbound.DialTarget{}, false
	}
	dt.CallerUser = cfg.CallerUser
	dt.CallerDisplayName = cfg.CallerDisplay
	return dt, true
}

// httpDialHostPortForVoicedialog maps HTTP listen addr (e.g. :8080, 0.0.0.0:8080) to a loopback host:port for ws dial.
func httpDialHostPortForVoicedialog(addr string) string {
	host, port, err := net.SplitHostPort("127.0.0.1" + addr)
	if err != nil {
		return "127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// Start wires outbound manager, SIP server, DB persistence, WebSeat hub, and starts UDP.
func Start(cfg Config) (*Embedded, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("sipapp: Config.DB is required")
	}
	acdDB := cfg.DB
	capTracker := server.NewTrunkCapacityTracker()

	persist.SetRegisterOutboundRequestURIServerPort(cfg.Port)
	// SDP c=/Call-ID host: cfg.LocalIP (e.g. cmd/server -sip-local-ip). Empty → 127.0.0.1 in server/outbound.
	localIP := strings.TrimSpace(cfg.LocalIP)

	sipHost := cfg.Host
	if sipHost == "0.0.0.0" {
		sipHost = localIP
	}
	warnIfSIPViaLoopback(sipHost, localIP, cfg.Host)

	var sipServerPtr *server.SIPServer
	var sipRegStore *persist.GormStore
	var sipCallPersist *persist.CallStore
	var campaignSvc *CampaignService

	// 主叫身份：优先从 Trunk + TrunkNumber 推导（数据库可见即生效），找不到再回退到 SIP_CALLER_ID / SIP_CALLER_DISPLAY_NAME。
	callerUser, callerDisplay := config.CallerIdentityFromEnv()
	if dbCfg, ok := models.PickTrunkTransferConfig(cfg.DB, 0); ok {
		if dbCfg.CallerUser != "" {
			callerUser = dbCfg.CallerUser
		}
		if dbCfg.CallerDisplay != "" {
			callerDisplay = dbCfg.CallerDisplay
		}
		if logger.Lg != nil {
			logger.Lg.Info("sipapp: using trunk-derived caller identity",
				zap.String("caller_user", callerUser),
				zap.String("caller_display", callerDisplay),
				zap.Uint("trunk_id", dbCfg.TrunkID),
				zap.Uint("trunk_number_id", dbCfg.TrunkNumberID),
			)
		}
	}
	outMgr := outbound.NewManager(outbound.ManagerConfig{
		LocalIP:         localIP,
		SIPHost:         sipHost,
		SIPPort:         cfg.Port,
		FromUser:        callerUser,
		FromDisplayName: callerDisplay,
		MediaAttach: func(ctx context.Context, cs *sipSession.CallSession) error {
			var voiceLog *zap.Logger
			if logger.Lg != nil {
				voiceLog = logger.Lg.Named("sip-voice")
			}
			return conversation.AttachVoicePipeline(ctx, cs, voiceLog)
		},
		OnRegisterSession: func(callID string, cs *sipSession.CallSession) {
			if sipServerPtr != nil {
				sipServerPtr.RegisterCallSession(callID, cs)
			}
		},
		OnTransferBridge: func(correlationID string, cs *sipSession.CallSession, outboundCallID string) {
			conversation.StartTransferBridge(correlationID, cs, outboundCallID, nil)
		},
		OnScript: func(ctx context.Context, leg outbound.EstablishedLeg, scriptID string) {
			if campaignSvc != nil {
				campaignSvc.RunScriptIfConfigured(ctx, leg, scriptID)
			}
		},
		OnEvent: func(evt outbound.DialEvent) {
			if evt.Scenario == outbound.ScenarioTransferAgent && evt.MediaProfile == outbound.MediaProfileTransferBridge {
				conversation.HandleTransferAgentDialEvent(evt)
			}
			if campaignSvc != nil {
				campaignSvc.HandleDialEvent(context.Background(), evt)
			}
		},
		OnEstablished: func(leg outbound.EstablishedLeg) {
			if campaignSvc != nil {
				campaignSvc.PrepareCallPrompt(leg.CallID, leg.CorrelationID)
			}
			if sipCallPersist == nil || leg.Session == nil {
				return
			}
			neg := leg.Session.NegotiatedCodec()
			rs := leg.Session.RTPSession()
			localRTP, remoteRTP := "", ""
			if rs != nil {
				if la := rs.LocalAddr; la != nil {
					localRTP = la.String()
				}
				if ra := rs.RemoteAddr; ra != nil {
					remoteRTP = ra.String()
				}
			}
			ctx := context.Background()
			var tenantID uint
			if campID, _, _, ok := parseCorrelation(leg.CorrelationID); ok && campID > 0 {
				if cm, err := models.GetSIPCampaignByID(ctx, acdDB, campID); err == nil {
					tenantID = cm.TenantID
				}
			}
			sipCallPersist.OnInvite(ctx, server.InvitePersistParams{
				TenantID:    tenantID,
				CallID:      leg.CallID,
				From:        leg.FromHeader,
				To:          leg.ToHeader,
				RemoteSig:   leg.RemoteSignalingAddr,
				RemoteRTP:   remoteRTP,
				LocalRTP:    localRTP,
				Codec:       neg.Name,
				PayloadType: neg.PayloadType,
				ClockRate:   neg.ClockRate,
				CSeqInvite:  leg.CSeqInvite,
				Direction:   "outbound",
			})
			sipCallPersist.OnEstablished(ctx, leg.CallID)
		},
	})

	sipServerPtr = server.New(server.Config{
		Host:          cfg.Host,
		Port:          cfg.Port,
		LocalIP:       localIP,
		OnSIPResponse: outMgr.HandleSIPResponse,
	})

	em := &Embedded{
		sipServer: sipServerPtr,
		outMgr:    outMgr,
	}

	campaignSvc = NewCampaignService(cfg.DB)
	sipRegStore = persist.NewGormStore(cfg.DB)
	campaignSvc.SetDialTargetResolver(func(ctx context.Context, phone string) (outbound.DialTarget, bool) {
		return sipRegStore.DialTargetForUsername(ctx, phone)
	})
	campaignSvc.StartWorker(outMgr)
	sipServerPtr.SetRegisterStore(sipRegStore)
	sipCallPersist = persist.NewCallStore(cfg.DB, logger.Lg)
	sipServerPtr.SetCallPersist(sipCallPersist)
	sipServerPtr.SetInboundDIDBindingResolver(func(msg *stack.Message) server.InboundDIDBinding {
		if acdDB == nil || msg == nil {
			return server.InboundDIDBinding{}
		}
		raw := server.InboundCalledPartyUser(msg)
		if raw == "" {
			return server.InboundDIDBinding{}
		}
		row, ok := models.FindTrunkNumberByInboundDID(acdDB, raw)
		if !ok {
			return server.InboundDIDBinding{}
		}
		if row.TenantID > 0 && logger.Lg != nil {
			logger.Lg.Info("sip inbound DID bound to trunk number",
				zap.String("called_user", raw),
				zap.Uint("tenant_id", row.TenantID),
				zap.Uint("trunk_number_id", row.ID))
		}
		return server.InboundDIDBinding{TenantID: row.TenantID, TrunkNumberID: row.ID}
	})
	sipServerPtr.SetInboundCapacityGate(func(callID, calledUser string) (bool, int, string) {
		if acdDB == nil {
			return true, 0, ""
		}
		raw := strings.TrimSpace(calledUser)
		if raw == "" {
			return true, 0, ""
		}
		row, ok := models.FindTrunkNumberByInboundDID(acdDB, raw)
		if !ok || row.CallInConcurrent == 0 {
			return true, 0, ""
		}
		if !capTracker.TryAcquireInbound(callID, row.ID, row.CallInConcurrent) {
			return false, 486, "Busy Here"
		}
		return true, 0, ""
	})
	sipServerPtr.SetInboundCapacityRelease(capTracker.ReleaseInbound)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SIP_INBOUND_ALLOW_UNKNOWN_DID"))) {
	case "1", "true", "yes", "on":
		sipServerPtr.SetInboundAllowUnknownDID(true)
	default:
		sipServerPtr.SetInboundAllowUnknownDID(false)
	}
	sipServerPtr.SetVoiceDialogWSLookup(func(callID string) string {
		cid := strings.TrimSpace(callID)
		if cid == "" || acdDB == nil {
			return ""
		}
		callRow, err := persist.FindActiveSIPCallByCallID(context.Background(), acdDB, cid)
		if err != nil {
			return ""
		}
		called := strings.TrimSpace(callRow.ToNumber)
		if called == "" {
			return ""
		}
		if tn, ok := models.FindTrunkNumberByInboundDID(acdDB, called); ok {
			return strings.TrimSpace(tn.VoiceDialogWSURL)
		}
		return ""
	})
	conversation.SetSIPTurnPersist(func(ctx context.Context, callID string, turn conversation.DialogTurn) {
		sipCallPersist.SaveConversationTurn(ctx, callID, turn)
	})
	conversation.SetTransferDialTargetResolver(func(ctx context.Context, inboundCallID string, exclude []uint) (outbound.DialTarget, bool) {
		return PickTransferDialTarget(ctx, acdDB, sipRegStore, inboundCallID, exclude)
	})
	conversation.SetTransferLegAbandoner(outMgr.AbandonEarlyTransferInvite)
	if logger.Lg != nil {
		logger.Lg.Info("sipapp: SIP persistence and campaign worker wired to application database pool")
	}
	em.campaignSvc = campaignSvc
	outMgr.BindSender(sipServerPtr)
	outMgr.SetOutboundCapacityRelease(capTracker.ReleaseOutbound)
	outMgr.SetDialGate(func(ctx context.Context, req outbound.DialRequest, callID string) error {
		if acdDB == nil {
			return nil
		}
		caller := strings.TrimSpace(req.CallerUser)
		if caller == "" {
			caller = strings.TrimSpace(req.Target.CallerUser)
		}
		if caller == "" {
			return nil
		}
		tenantID := req.DialTenantID
		if tenantID == 0 {
			cid := strings.TrimSpace(req.CorrelationID)
			if cid != "" {
				row, err := persist.FindActiveSIPCallByCallID(ctx, acdDB, cid)
				if err == nil && row.TenantID > 0 {
					tenantID = row.TenantID
				}
			}
		}
		if tenantID == 0 {
			return nil
		}
		tn, ok := models.FindTrunkNumberForOutboundCaller(acdDB, tenantID, caller)
		if !ok || tn.Concurrent == 0 {
			return nil
		}
		if !capTracker.TryAcquireOutbound(callID, tn.ID, tn.Concurrent) {
			return fmt.Errorf("outbound concurrent limit exceeded for CLI %q (limit=%d)", caller, tn.Concurrent)
		}
		return nil
	})
	conversation.SetTransferDialer(outMgr)
	conversation.SetInboundSessionLookup(func(callID string) *sipSession.CallSession {
		if sipServerPtr == nil {
			return nil
		}
		return sipServerPtr.GetCallSession(callID)
	})
	conversation.SetCallStore(sipServerPtr)
	conversation.SetTransferPeerCallbacks(outMgr.SendBYE, sipServerPtr.SendUASBye)
	conversation.SetSIPHangup(func(callID string) {
		callID = strings.TrimSpace(callID)
		if callID == "" {
			return
		}
		if err := outMgr.SendBYE(callID); err == nil {
			if logger.Lg != nil {
				logger.Lg.Info("sip: hangup outbound BYE sent", zap.String("call_id", callID))
			}
			sipServerPtr.HangupInboundCall(callID)
			return
		}
		sipServerPtr.HangupInboundCall(callID)
	})
	webseat.InitDefault(webseat.Config{
		RemoveCallSession:     sipServerPtr.RemoveCallSession,
		ForgetUASDialog:       sipServerPtr.ForgetUASDialog,
		SendUASBye:            sipServerPtr.SendUASBye,
		ReleaseTransferDedupe: conversation.ReleaseTransferStartDedupe,
		OnWebSeatBridgeEstablished: func(callID string) {
			conversation.ResetTransferRoutingState(callID)
		},
		OnWebSeatJoinTimeout: conversation.OnWebSeatJoinTimeout,
		SetACDWebSeatWorkState: func(ctx context.Context, targetID uint, workState string) error {
			if acdDB == nil || targetID == 0 {
				return nil
			}
			return models.UpdateACDPoolTargetWorkState(ctx, acdDB, targetID, workState, "sip")
		},
		FinalizeInboundPersist: func(ctx context.Context, callID, initiator string, raw []byte, codecName string, recordSampleRate, recordOpusChannels int) {
			if sipCallPersist == nil {
				return
			}
			sipCallPersist.OnBye(ctx, server.ByePersistParams{
				CallID:             callID,
				RawPayload:         raw,
				CodecName:          codecName,
				Initiator:          initiator,
				RecordSampleRate:   recordSampleRate,
				RecordOpusChannels: recordOpusChannels,
			})
		},
	})
	conversation.SetWebSeatTransfer(conversation.StartWebSeatHandoff)
	useTLS := config.GlobalConfig.Server.SSLEnabled
	loopDialHostPort := httpDialHostPortForVoicedialog(config.GlobalConfig.Server.Addr)
	voicedialog.InitDefault(voicedialog.Config{
		HangupInbound: func(callID string) {
			if sipServerPtr != nil {
				sipServerPtr.HangupInboundCall(callID)
			}
		},
		InboundLoopbackWS:             true,
		LoopbackUseTLS:                useTLS,
		LoopbackTLSInsecureSkipVerify: false,
		LoopbackHTTPHostPort:          loopDialHostPort,
		APIPrefix:                     config.GlobalConfig.Server.APIPrefix,
	})

	logger.Info("sipapp: inbound SIP legs use voicedialog WebSocket bridge (HTTP); outbound AI uses embedded pipeline",
		zap.Bool("voicedialog_inbound_loopback_ws", true),
		zap.String("voicedialog_loopback_dial_host_port", loopDialHostPort),
	)
	if err := sipServerPtr.Start(); err != nil {
		return nil, fmt.Errorf("sipapp: sip start: %w", err)
	}
	if logger.Lg != nil {
		logger.Lg.Info("sipapp: SIP UDP listening",
			zap.String("host", cfg.Host),
			zap.Int("port", cfg.Port),
			zap.String("local_ip_effective", localIP),
			zap.String("local_ip_from_cli", strings.TrimSpace(cfg.LocalIP)),
		)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "sipapp: listening on udp %s:%d (SDP local-ip effective=%q cli=%q)\n", cfg.Host, cfg.Port, localIP, strings.TrimSpace(cfg.LocalIP))
	}
	if dt, ok := resolveOutboundDialTarget(cfg.DB, sipRegStore); ok {
		if logger.Lg != nil {
			logger.Lg.Info("sipapp: outbound target ready",
				zap.String("uri", dt.RequestURI),
				zap.String("signaling", dt.SignalingAddr),
			)
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "sipapp: outbound target ready: uri=%s signaling=%s\n", dt.RequestURI, dt.SignalingAddr)
		}
	} else {
		if utils.GetEnv(constants.EnvSIPTargetNumber) != "" {
			_, _ = fmt.Fprintf(os.Stderr, "sipapp: SIP_TARGET_NUMBER 已设置但找不到外呼目标；请在「中继线路 / 中继号码」里给某条号码勾选 direction ∈ {outbound, both}。详见 docs/SIP_OUTBOUND_MODULE.md\n")
		}
	}

	return em, nil
}

// resolveInboundTrunkNumberPK maps called-party digits (sip_calls.to_number) to sip_trunk_numbers.id for ACD trunk scope.
func resolveInboundTrunkNumberPK(db *gorm.DB, calledUser string) uint {
	if db == nil {
		return 0
	}
	raw := strings.TrimSpace(calledUser)
	if raw == "" {
		return 0
	}
	if tn, ok := models.FindTrunkNumberByInboundDID(db, raw); ok {
		return tn.ID
	}
	return 0
}

// Shutdown stops the campaign worker and SIP UDP.
func (e *Embedded) Shutdown(ctx context.Context) {
	if e == nil {
		return
	}
	if logger.Lg != nil {
		logger.Lg.Info("sipapp: shutting down")
	} else {
		_, _ = fmt.Fprintln(os.Stdout, "sipapp: shutting down...")
	}
	if e.campaignSvc != nil {
		e.campaignSvc.StopWorker()
	}
	if e.sipServer != nil {
		_ = e.sipServer.Stop()
	}
}

// PickTransferDialTarget selects one row from acd_pool_targets for blind transfer (DTMF).
// Eligible: not deleted, weight > 0, work_state = available, route_type sip or web.
// Ordering: weight DESC, id ASC (highest weight wins; tie-break lower id first).
//   - web → WebSeat (browser agent leg).
//   - sip trunk → DialTargetFromACDTrunk; sip internal → reg.DialTargetForUsername.
//
// Blind transfer targets come only from acd_pool_targets (plus trunk-level SIP fields on each pool row).
// There is no env-based fallback dial string (configure Web/SIP rows in the pool).
func PickTransferDialTarget(ctx context.Context, db *gorm.DB, reg *persist.GormStore, inboundCallID string, exclude []uint) (outbound.DialTarget, bool) {
	if db == nil {
		return outbound.DialTarget{}, false
	}
	var tenantID uint
	var calledUser string
	if cid := strings.TrimSpace(inboundCallID); cid != "" {
		if call, err := persist.FindSIPCallByCallID(ctx, db, cid); err == nil {
			tenantID = call.TenantID
			calledUser = strings.TrimSpace(call.ToNumber)
		}
	}
	inboundTrunkNumberID := resolveInboundTrunkNumberPK(db, calledUser)
	row, err := models.PickEligibleACDPoolTargetForTransfer(ctx, db, exclude, tenantID, inboundTrunkNumberID)
	if err != nil {
		return outbound.DialTarget{}, false
	}

	if row.RouteType == models.ACDPoolRouteTypeWeb {
		if strings.TrimSpace(inboundCallID) != "" {
			if err := models.UpdateACDPoolTargetWorkState(ctx, db, row.ID, models.ACDWorkStateRinging, "sip-transfer"); err == nil {
				webseat.BindInboundCallToWebACD(strings.TrimSpace(inboundCallID), row.ID)
			}
		}
		return outbound.DialTarget{WebSeat: true, ACDPoolTargetID: row.ID}, true
	}

	var dt outbound.DialTarget
	src := strings.ToLower(strings.TrimSpace(row.SipSource))
	switch src {
	case models.ACDSipSourceTrunk:
		t, ok := outbound.DialTargetFromACDTrunk(row.TargetValue, row.SipTrunkHost, row.SipTrunkSignalingAddr, row.SipTrunkPort)
		if !ok {
			return outbound.DialTarget{}, false
		}
		dt = t
	default:
		if reg == nil {
			return outbound.DialTarget{}, false
		}
		u := strings.TrimSpace(row.TargetValue)
		if u == "" {
			return outbound.DialTarget{}, false
		}
		t, ok := reg.DialTargetForUsername(ctx, u)
		if !ok {
			return outbound.DialTarget{}, false
		}
		dt = t
	}
	dt.CallerUser = strings.TrimSpace(row.SipCallerID)
	dt.CallerDisplayName = strings.TrimSpace(row.SipCallerDisplayName)
	// 即便 ACD 行命中了，也允许 Trunk 信息补齐缺失字段（例如运营忘了在 ACD 行上填 caller_id）。
	if dt.CallerUser == "" || dt.CallerDisplayName == "" {
		if tc, ok := models.PickTrunkTransferConfig(db, tenantID); ok {
			if dt.CallerUser == "" {
				dt.CallerUser = tc.CallerUser
			}
			if dt.CallerDisplayName == "" {
				dt.CallerDisplayName = tc.CallerDisplay
			}
		}
	}
	dt.ACDPoolTargetID = row.ID
	return dt, true
}
