package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/recognizer"
)

type asrOutcome struct {
	text string
	err  error
}

// SentenceNotify is called on each Tencent sentence end (fragment = 本句增量, cumulative = 累计转写).
type SentenceNotify func(fragment string, cumulative string)

// TranscribePCM runs Tencent real-time ASR over raw s16le mono PCM (no per-sentence callback).
func TranscribePCM(ctx context.Context, pcm []byte, env Env, sampleRateHz int) (string, error) {
	return transcribePCM(ctx, pcm, env, sampleRateHz, nil)
}

// TranscribePCMWithSentenceNotify is like TranscribePCM but invokes notify on each sentence boundary.
func TranscribePCMWithSentenceNotify(ctx context.Context, pcm []byte, env Env, sampleRateHz int, notify SentenceNotify) (string, error) {
	return transcribePCM(ctx, pcm, env, sampleRateHz, notify)
}

// transcribePCM feeds PCM at 1× real-time (see PCMSampleRateFromASRModel + DecodeToPCMSMono).
func transcribePCM(ctx context.Context, pcm []byte, env Env, sampleRateHz int, sentenceNotify SentenceNotify) (string, error) {
	if len(pcm) == 0 {
		return "", fmt.Errorf("asr: empty pcm")
	}
	if sampleRateHz <= 0 {
		sampleRateHz = 16000
	}
	opt := recognizer.NewQcloudASROption(env.ASRAppID, env.ASRSecretID, env.ASRSecretKey)
	if strings.TrimSpace(env.ASRModelType) != "" {
		opt.ModelType = strings.TrimSpace(env.ASRModelType)
	}
	if sentenceNotify != nil {
		opt.SentenceNotify = func(frag, cum string) {
			sentenceNotify(frag, cum)
		}
	}
	svc := recognizer.NewQcloudASR(opt)

	outcomeCh := make(chan asrOutcome, 1)
	var once sync.Once
	finish := func(t string, e error) {
		once.Do(func() {
			outcomeCh <- asrOutcome{text: strings.TrimSpace(t), err: e}
		})
	}

	var mu sync.Mutex
	transcript := ""

	svc.Init(func(text string, isLast bool, _ time.Duration, _ string) {
		if text != "" {
			mu.Lock()
			transcript = strings.TrimSpace(text)
			mu.Unlock()
		}
		if isLast {
			mu.Lock()
			t := transcript
			mu.Unlock()
			finish(t, nil)
		}
	}, func(err error, fatal bool) {
		if fatal {
			finish("", err)
		}
	})

	if err := svc.ConnAndReceive("call-analysis"); err != nil {
		_ = svc.StopConn()
		return "", fmt.Errorf("asr connect: %w", err)
	}
	defer func() { _ = svc.StopConn() }()

	bytesPerSec := sampleRateHz * 2 // s16 mono
	chunkBytes := bytesPerSec / 50
	if chunkBytes < 320 {
		chunkBytes = 320
	}

	pcmAudioDur := time.Duration(int64(len(pcm)) * int64(time.Second) / int64(bytesPerSec))
	completeWait := pcmAudioDur + 3*time.Minute
	if completeWait < 90*time.Second {
		completeWait = 90 * time.Second
	}
	const maxCompleteWait = 2 * time.Hour
	if completeWait > maxCompleteWait {
		completeWait = maxCompleteWait
	}
	timer := time.NewTimer(completeWait)
	defer timer.Stop()

	for i := 0; i < len(pcm); i += chunkBytes {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		end := i + chunkBytes
		if end > len(pcm) {
			end = len(pcm)
		}
		n := end - i
		if err := svc.SendAudioBytes(pcm[i:end]); err != nil {
			return "", fmt.Errorf("asr send: %w", err)
		}
		sleep := time.Duration(int64(n) * int64(time.Second) / int64(bytesPerSec))
		if sleep > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(sleep):
			}
		}
	}
	if err := svc.SendEnd(); err != nil {
		return "", fmt.Errorf("asr end: %w", err)
	}

	select {
	case o := <-outcomeCh:
		return o.text, o.err
	case <-timer.C:
		mu.Lock()
		t := transcript
		mu.Unlock()
		if t != "" {
			return t, fmt.Errorf("asr: timeout without final callback (partial transcript only)")
		}
		return "", fmt.Errorf("asr: timeout waiting for recognition")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
