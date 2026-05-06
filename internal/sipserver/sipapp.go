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

func resolveOutboundDialTarget(store *persist.GormStore) (outbound.DialTarget, bool) {
	if store != nil {
		n := utils.GetEnv(constants.EnvSIPTargetNumber)
		if n != "" {
			if dt, ok := store.DialTargetForUsername(context.Background(), n); ok {
				return dt, true
			}
		}
	}
	return outbound.DialTargetFromEnv()
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

	callerUser, callerDisplay := config.CallerIdentityFromEnv()
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
			sipCallPersist.OnInvite(ctx, server.InvitePersistParams{
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
	if dt, ok := resolveOutboundDialTarget(sipRegStore); ok {
		if logger.Lg != nil {
			logger.Lg.Info("sipapp: outbound target from env",
				zap.String("uri", dt.RequestURI),
				zap.String("signaling", dt.SignalingAddr),
			)
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "sipapp: outbound target from env: uri=%s signaling=%s\n", dt.RequestURI, dt.SignalingAddr)
		}
	} else {
		if utils.GetEnv(constants.EnvSIPTargetNumber) != "" {
			_, _ = fmt.Fprintf(os.Stderr, "sipapp: SIP_TARGET_NUMBER is set but outbound target is incomplete; set SIP_OUTBOUND_HOST (and optionally SIP_OUTBOUND_PORT, SIP_SIGNALING_ADDR). See docs/SIP_OUTBOUND_MODULE.md\n")
		}
	}

	return em, nil
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
func PickTransferDialTarget(ctx context.Context, db *gorm.DB, reg *persist.GormStore, inboundCallID string, exclude []uint) (outbound.DialTarget, bool) {
	if db == nil {
		return outbound.DialTarget{}, false
	}
	row, err := models.PickEligibleACDPoolTargetForTransfer(ctx, db, exclude)
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
	dt.ACDPoolTargetID = row.ID
	return dt, true
}
