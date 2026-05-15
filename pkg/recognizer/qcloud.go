package recognizer

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/media"
	gonanoid "github.com/matoous/go-nanoid"
	"github.com/sirupsen/logrus"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/asr"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/common"
)

type QCloudASR struct {
	Handler     media.MediaHandler
	sentence    string
	sliceType   uint32
	startTime   uint32
	endTime     uint32
	sendReqTime *time.Time
	endReqTime  *time.Time

	opt              QCloudASROption
	recognizer       *asr.SpeechRecognizer
	transcribeResult TranscribeResult
	processError     ProcessError
	dialogID         string

	// lastFinalEmitted is cumulative text last sent with isLast=true from OnSentenceEnd
	// (used to skip a duplicate final from OnRecognitionComplete).
	lastFinalEmitted string
}

type QCloudASROption struct {
	AppID       string    `json:"appId" yaml:"app_id" env:"QCLOUD_APP_ID"`
	SecretID    string    `json:"secretId" yaml:"secret_id" env:"QCLOUD_SECRET_ID"`
	SecretKey   string    `json:"secret" yaml:"secret" env:"QCLOUD_SECRET"`
	Format      int       `json:"format" yaml:"format" default:"1"`
	ModelType   string    `json:"modelType" yaml:"model_type" env:"QCLOUD_MODEL_TYPE" default:"16k_zh"`
	ReqChanSize int       `json:"reqChanSize" yaml:"req_chan_size" default:"128"`
	HotWords    []HotWord `json:"hotWords" yaml:"hot_words"`
	// VadSilenceTime 单位 ms，腾讯云 ASR 触发 sentence-end 的静默阈值。
	// SDK 不显式设置时走云端默认（约 800~1000ms）。降到 400ms 可显著缩短
	// 用户停顿到 asr.final 的间隔；过低（<300ms）容易把语气词当切句。
	// 取值范围：240~2000。0 表示走 SDK 默认。
	VadSilenceTime int `json:"vadSilenceTime" yaml:"vad_silence_time" default:"400"`
	// FilterDirty=1 过滤脏词；FilterModal=2 过滤强语气词；NeedVad=1 启用 vad 切句
	// （腾讯云 vad_silence_time 必须配合 needvad=1 才生效）。
	FilterDirty int `json:"filterDirty" yaml:"filter_dirty" default:"0"`
	FilterModal int `json:"filterModal" yaml:"filter_modal" default:"0"`
	NeedVad     int `json:"needVad" yaml:"need_vad" default:"1"`
	// SentenceNotify is invoked on each Tencent OnSentenceEnd (fragment = 本句增量, cumulative = 当前累计).
	// Optional; used by offline streaming consumers (e.g. call analysis WebSocket).
	SentenceNotify func(fragment string, cumulative string) `json:"-" yaml:"-"`
}

func NewQcloudASROption(appId string, secretId string, secretKey string) QCloudASROption {
	return QCloudASROption{
		AppID:       appId,
		SecretID:    secretId,
		SecretKey:   secretKey,
		Format:      asr.AudioFormatPCM,
		ModelType:   "16k_zh",
		ReqChanSize: 128,
	}
}

func WithQCloudASR(opt QCloudASROption) media.MediaHandlerFunc {
	executor := media.NewAsyncTaskRunner[[]byte](opt.ReqChanSize)
	credential := common.NewCredential(opt.SecretID, opt.SecretKey)

	asq := &QCloudASR{opt: opt}
	recognizer := asr.NewSpeechRecognizer(opt.AppID, credential, opt.ModelType, asq)
	recognizer.VoiceFormat = opt.Format
	applyQCloudASRTuning(recognizer, opt)

	executor.ConcurrentMode = false // QCloud ASR write is not blocking so we need to set this to false
	executor.RequestBuilder = func(h media.MediaHandler, packet media.MediaPacket) (*media.PacketRequest[[]byte], error) {
		audioPacket, ok := packet.(*media.AudioPacket)
		if !ok {
			h.EmitPacket(asq, packet)
			return nil, nil
		}
		if asq.Handler == nil {
			asq.Handler = h
		}
		req := media.PacketRequest[[]byte]{
			Req:       audioPacket.Payload,
			Interrupt: true,
		}
		return &req, nil
	}

	executor.InitCallback = func(h media.MediaHandler) error {
		asq.Handler = h
		return recognizer.Start()
	}

	executor.TerminateCallback = func(h media.MediaHandler) error {
		err := recognizer.Stop()
		if err != nil && err.Error() == "recognizer is not running" {
			return nil
		}
		return err
	}

	executor.StateCallback = func(h media.MediaHandler, event media.StateChange) error {
		switch event.State {
		case media.Hangup:
			err := recognizer.Stop()
			if err != nil && err.Error() == "recognizer is not running" {
				return nil
			}
			return err
		case media.StartSilence:
			n := time.Now()
			asq.endReqTime = &n
		case media.StartSpeaking:
			n := time.Now()
			asq.sendReqTime = &n
		}
		return nil
	}

	executor.TaskExecutor = func(ctx context.Context, h media.MediaHandler, req media.PacketRequest[[]byte]) error {
		if asq.sendReqTime == nil {
			n := time.Now()
			asq.sendReqTime = &n
			logrus.Info("qcloud asr: start send request")
		}
		return recognizer.Write(req.Req)
	}
	return executor.HandleMediaData
}

func (opt QCloudASROption) String() string {
	return fmt.Sprintf("QCloudASROption{AppID: %s, Format: %d, ModelType: %s, ReqChanSize: %d}",
		opt.AppID, opt.Format, opt.ModelType, opt.ReqChanSize)
}

// OnRecognitionStart implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnRecognitionStart(response *asr.SpeechRecognitionResponse) {
	asq.lastFinalEmitted = ""
	logFields := logrus.Fields{
		"voice_id": response.VoiceID,
	}
	if asq.Handler != nil {
		logFields["handler"] = asq.Handler
	}
	logrus.WithFields(logFields).Info("OnRecognitionStart")
}

// OnSentenceBegin implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnSentenceBegin(response *asr.SpeechRecognitionResponse) {
	sendReqTime := time.Now()
	asq.sendReqTime = &sendReqTime
}

// OnRecognitionResultChange implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnRecognitionResultChange(response *asr.SpeechRecognitionResponse) {
	if asq.transcribeResult != nil {
		asq.transcribeResult(response.Result.VoiceTextStr, false, time.Since(*asq.sendReqTime), asq.dialogID)
		return
	}
}

// OnSentenceEnd implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnSentenceEnd(response *asr.SpeechRecognitionResponse) {
	logFields := logrus.Fields{
		"voiceTextStr": response.Result.VoiceTextStr,
	}
	if asq.Handler != nil {
		logFields["sessionID"] = asq.Handler.GetSession().ID
	}
	logrus.WithFields(logFields).Info("qcloud: on sentence end")

	asq.sentence += response.Result.VoiceTextStr
	asq.sliceType = response.Result.SliceType
	asq.startTime = response.Result.StartTime
	asq.endTime = response.Result.EndTime
	if asq.opt.SentenceNotify != nil {
		asq.opt.SentenceNotify(response.Result.VoiceTextStr, asq.sentence)
	}
	if asq.transcribeResult != nil {
		// Sentence boundary: treat as final for streaming consumers (SIP / WebSocket).
		// Previously this passed false, so only OnRecognitionComplete emitted isLast=true
		// (often at hangup), and all sentence-end logs looked like endless partials.
		asq.transcribeResult(asq.sentence, true, time.Since(*asq.sendReqTime), asq.dialogID)
		asq.lastFinalEmitted = asq.sentence
		// Do not carry prior sentences into the next final (asq.sentence was += across boundaries).
		asq.sentence = ""
		return
	}
}

// OnRecognitionComplete implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnRecognitionComplete(response *asr.SpeechRecognitionResponse) {
	finalSentence := asq.sentence
	asq.sentence = ""
	asq.sliceType = 0
	logFields := logrus.Fields{
		"voiceTextStr":  response.Result.VoiceTextStr,
		"finalSentence": finalSentence,
	}
	if asq.Handler != nil {
		logFields["sessionID"] = asq.Handler.GetSession().ID
	}
	logrus.WithFields(logFields).Info("qcloud: on sentence complete")

	// 优先使用 transcribeResult 回调
	if asq.transcribeResult != nil {
		t := strings.TrimSpace(finalSentence)
		if t == "" {
			t = strings.TrimSpace(response.Result.VoiceTextStr)
		}
		last := strings.TrimSpace(asq.lastFinalEmitted)
		// Last sentence often already emitted in OnSentenceEnd; avoid duplicate session-end.
		if t != "" && t != last {
			asq.transcribeResult(t, true, time.Since(*asq.sendReqTime), asq.dialogID)
		}
		asq.lastFinalEmitted = ""
		return
	}

	// 如果没有 transcribeResult 回调，尝试使用 Handler
	if asq.Handler != nil {
		packet := &media.TextPacket{
			Text:          finalSentence,
			IsTranscribed: true,
		}
		asq.Handler.EmitPacket(asq.Handler, packet)
		asq.Handler.EmitState(asq, media.Completed, &media.CompletedData{
			SenderName: "asr.qcloud",
			Result:     finalSentence,
			Duration:   time.Since(*asq.sendReqTime),
		})
	}
}

// OnFail implementation of SpeechRecognitionListener
func (asq *QCloudASR) OnFail(response *asr.SpeechRecognitionResponse, err error) {
	if response.Code == 4008 {
		// no audio data send error
		return
	}
	if strings.Contains(err.Error(), "EOF") {
		logFields := logrus.Fields{
			"voice_id": response.VoiceID,
			"error":    err,
		}
		if asq.Handler != nil {
			logFields["handler"] = asq.Handler
		}
		logrus.WithFields(logFields).Warn("qcloud: eof onfail")
		return
	}
	logFields := logrus.Fields{
		"voice_id": response.VoiceID,
		"error":    err,
	}
	if asq.Handler != nil {
		logFields["handler"] = asq.Handler
	}
	logrus.WithFields(logFields).Error("OnFail")

	// 优先使用 processError 回调
	if asq.processError != nil {
		asq.processError(err, true)
		return
	}

	// 如果没有 processError 回调，尝试使用 Handler
	if asq.Handler != nil {
		asq.Handler.CauseError(asq, err)
	}
}

func NewQcloudASR(opt QCloudASROption) *QCloudASR {
	asq := &QCloudASR{opt: opt}
	return asq
}

func (asq *QCloudASR) Init(tr TranscribeResult, er ProcessError) {
	asq.transcribeResult = tr
	asq.processError = er
}

func (asq *QCloudASR) Vendor() string {
	return "qcloud"
}

// applyQCloudASRTuning 把 LingEchoX 暴露的 QCloudASROption 翻译成腾讯云
// SpeechRecognizer 的查询参数。所有字段都是可选；为 0 时走 SDK 默认。
func applyQCloudASRTuning(r *asr.SpeechRecognizer, opt QCloudASROption) {
	if r == nil {
		return
	}
	if opt.VadSilenceTime > 0 {
		// 腾讯云协议要求 240~2000，超出范围就交给云端拒绝。
		r.VadSilenceTime = opt.VadSilenceTime
	}
	if opt.NeedVad > 0 {
		r.NeedVad = opt.NeedVad
	}
	if opt.FilterDirty > 0 {
		r.FilterDirty = opt.FilterDirty
	}
	if opt.FilterModal > 0 {
		r.FilterModal = opt.FilterModal
	}
}

func (asq *QCloudASR) ConnAndReceive(dialogID string) error {
	asq.dialogID = dialogID
	credential := common.NewCredential(asq.opt.SecretID, asq.opt.SecretKey)
	recognizer := asr.NewSpeechRecognizer(asq.opt.AppID, credential, asq.opt.ModelType, asq)
	recognizer.VoiceFormat = asq.opt.Format
	applyQCloudASRTuning(recognizer, asq.opt)
	hotWords := asq.opt.HotWords

	var hotWordsStr string
	for _, hotWord := range hotWords {
		var weight string
		if hotWord.Weight > 0 {
			weight = fmt.Sprintf("%d", hotWord.Weight)
		} else {
			weight = "10"
		}
		wordStr := hotWord.Word + "|" + weight
		hotWordsStr += wordStr + ","
	}
	recognizer.HotwordList = strings.TrimSuffix(hotWordsStr, ",")
	if len(hotWordsStr) > 0 {
		logFields := logrus.Fields{
			"hotwords": recognizer.HotwordList,
		}
		if asq.Handler != nil {
			logFields["sessionID"] = asq.Handler.GetSession().ID
		}
		logrus.WithFields(logFields).Info("qcloud: hotwords")
	}
	err := recognizer.Start()
	if err != nil {
		logrus.WithError(err).Error("qcloud: recognizer.Start")
	}
	asq.recognizer = recognizer
	now := time.Now()
	asq.sendReqTime = &now
	asq.endReqTime = &now
	return nil
}

func (asq *QCloudASR) Activity() bool {
	return asq.recognizer != nil
}

func (asq *QCloudASR) RestartClient() {
	_ = asq.StopConn()
	dialogID, _ := gonanoid.Nanoid()
	_ = asq.ConnAndReceive(dialogID)
}

func (asq *QCloudASR) SendAudioBytes(data []byte) error {
	if asq.recognizer == nil || data == nil {
		return nil
	}
	return asq.recognizer.Write(data)
}

func (asq *QCloudASR) SendEnd() error {
	if asq.recognizer != nil {
		_ = asq.recognizer.Stop()
		asq.recognizer = nil
	}
	return nil
}

func (asq *QCloudASR) StopConn() error {
	if asq.recognizer != nil {
		_ = asq.recognizer.Stop()
		asq.recognizer = nil
	}
	return nil
}
