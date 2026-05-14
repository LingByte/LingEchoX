package synthesizer

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// QCloud TTS via WebSocket (`/stream_ws`, action=TextToStreamAudioWS).
//
// Why a second method on QCloudService?
//
// 我们当前用的 `tts.SpeechSynthesizer` 走 HTTPS POST：每次请求都要 TLS 握手 +
// 发送整段文本 + 服务端用 HTTP chunk 编码回流 PCM。实测从 `Synthesis(text)` 到首块
// PCM 大约 ~300~500ms。
//
// 腾讯云 Speech SDK 还提供另一个入口 `tts.SpeechWsSynthesizer`，走 WebSocket。
// 协议依旧是「一连一句」（不能在已有连接上追加文本），但服务端用 binary frame
// 直接 push PCM，省掉 HTTP chunk 编码 / Transfer-Encoding 的开销，**首字节通常
// 比 HTTPS 快 50~150ms**。这是当前 SDK 能给的最低延迟路径。
//
// 真正的「同一会话增量喂文本」腾讯云在该 SDK 里没有暴露——如果以后开了实时语
// 音合成 RTSpeech，再加 `qcloud_rt.go` 即可。

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/tencentcloud/tencentcloud-speech-sdk-go/common"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/tts"
)

// SynthesizeStream 用 WebSocket TTS 合成 text，PCM 通过 callback 流式回调。
//
// callback 在 SDK 的 dispatcher goroutine 上调用；回调返回 error 会被记录但不会
// 主动断开 WS（服务端自然完成或 ctx 取消才会断）。ctx 取消会主动 CloseConn，让
// SDK 的 Wait() 立即返回。
func (qs *QCloudService) SynthesizeStream(ctx context.Context, text string, callback func(pcm []byte) error) error {
	if qs == nil {
		return fmt.Errorf("qcloud tts: nil service")
	}
	if callback == nil {
		return fmt.Errorf("qcloud tts: nil callback")
	}
	qs.mu.Lock()
	opt := qs.opt
	qs.mu.Unlock()

	listener := &qcloudWsListener{
		ctx:      ctx,
		callback: callback,
	}
	credential := common.NewCredential(opt.SecretID, opt.SecretKey)
	synth := tts.NewSpeechWsSynthesizer(opt.AppID, credential, listener)
	synth.VoiceType = opt.VoiceType
	if opt.SampleRate > 0 {
		synth.SampleRate = int64(opt.SampleRate)
	}
	if opt.Codec != "" {
		synth.Codec = opt.Codec
	} else {
		synth.Codec = "pcm"
	}
	synth.Text = text
	applyQCloudWSTTSSpeed(synth, opt.Speed)

	// Watcher: cancel ctx → close ws so SDK Wait() returns immediately.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			synth.CloseConn()
		case <-done:
		}
	}()

	if err := synth.Synthesis(); err != nil {
		return fmt.Errorf("qcloud ws tts: %w", err)
	}
	_ = synth.Wait()

	listener.mu.Lock()
	failErr := listener.err
	listener.mu.Unlock()
	if failErr != nil && !errors.Is(failErr, context.Canceled) {
		// If ctx is already cancelled, surface ctx.Err() so callers can distinguish
		// barge-in from real upstream errors.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// applyQCloudWSTTSSpeed handles the SDK quirk that SpeechWsSynthesizer.Speed is
// declared as float64 (vs int64 on the HTTP variant). Use reflection so we are
// resilient to SDK upgrades flipping the type back.
func applyQCloudWSTTSSpeed(synth *tts.SpeechWsSynthesizer, speed int64) {
	if synth == nil || speed == 0 {
		return
	}
	rv := reflect.ValueOf(synth)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	ev := rv.Elem()
	if !ev.IsValid() {
		return
	}
	f := ev.FieldByName("Speed")
	if !f.IsValid() || !f.CanSet() {
		return
	}
	switch f.Kind() {
	case reflect.Float32, reflect.Float64:
		f.SetFloat(float64(speed))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(speed)
	}
}

type qcloudWsListener struct {
	ctx      context.Context
	callback func(pcm []byte) error
	mu       sync.Mutex
	err      error
}

func (l *qcloudWsListener) OnSynthesisStart(*tts.SpeechWsSynthesisResponse) {}
func (l *qcloudWsListener) OnSynthesisEnd(*tts.SpeechWsSynthesisResponse)   {}
func (l *qcloudWsListener) OnTextResult(*tts.SpeechWsSynthesisResponse)     {}

func (l *qcloudWsListener) OnAudioResult(data []byte) {
	if l == nil || len(data) == 0 {
		return
	}
	if l.ctx != nil && l.ctx.Err() != nil {
		return
	}
	if err := l.callback(data); err != nil {
		l.mu.Lock()
		if l.err == nil {
			l.err = err
		}
		l.mu.Unlock()
	}
}

func (l *qcloudWsListener) OnSynthesisFail(_ *tts.SpeechWsSynthesisResponse, err error) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.err == nil {
		l.err = err
	}
	l.mu.Unlock()
}
