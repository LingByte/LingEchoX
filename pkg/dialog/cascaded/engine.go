// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// ErrAlreadyAttached is returned when Attach is invoked twice on the
// same Engine instance. One Engine serves exactly one call; reusing
// the same instance for a second call indicates a caller bug.
var ErrAlreadyAttached = errors.New("dialog/cascaded: engine already attached")

// Engine is the native cascaded engine. It runs a pipeline.Pipeline
// (ASR → LLM → TTS) over the audio stream surfaced by a MediaPort.
//
// PR-8c: the stage implementations are passthrough stubs; see doc.go.
// The lifecycle plumbing (attach / drain / detach / idempotency) is
// production-shaped and will not change when real providers land.
type Engine struct {
	cfg    engine.Config
	stages []pipeline.Stage

	attached  atomic.Bool // single-shot Attach guard
	detachOnce sync.Once  // idempotent Detach guard

	// State filled in by Attach for Detach to consume.
	cancel context.CancelFunc
	done   chan struct{}
	pipeErr error
}

// New builds an Engine for the supplied Config using the default
// stage list. Stage swapping happens in factory.go via Build() once
// real providers are wired in.
func New(cfg engine.Config) *Engine {
	return &Engine{
		cfg:    cfg,
		stages: defaultStages(),
	}
}

// Mode reports the static engine mode. Always engine.ModeCascaded.
func (e *Engine) Mode() engine.Mode { return engine.ModeCascaded }

// Attach binds the engine to one MediaPort. Returns quickly; the
// pipeline runs in goroutines owned by the engine until Detach (or
// ctx cancellation) tears them down.
//
// Concurrency contract (matches engine.Engine):
//
//   - Attach is called once per Engine instance. Calling it twice
//     returns ErrAlreadyAttached.
//   - The returned Detach is safe to call from any goroutine and is
//     idempotent — subsequent calls are no-ops.
//   - Cancelling ctx is equivalent to invoking Detach.
func (e *Engine) Attach(ctx context.Context, port engine.MediaPort, lg engine.Logger) (engine.Detach, error) {
	if !e.attached.CompareAndSwap(false, true) {
		return nil, ErrAlreadyAttached
	}
	if port == nil {
		return nil, fmt.Errorf("dialog/cascaded: nil MediaPort")
	}
	if lg == nil {
		lg = engine.NopLogger{}
	}
	lg = lg.With(
		engine.F("engine", "cascaded"),
		engine.F("call_id", e.cfg.CallID),
		engine.F("tenant_id", e.cfg.TenantID),
	)
	lg.Info("cascaded engine: attaching",
		engine.F("stages", len(e.stages)))

	pipe, err := pipeline.New("cascaded", e.stages)
	if err != nil {
		return nil, fmt.Errorf("dialog/cascaded: build pipeline: %w", err)
	}

	// Engine-owned context. ctx-cancel from the caller AND Detach
	// both trip this; whichever fires first wins.
	engCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.done = make(chan struct{})

	// PCM-in bridge: MediaPort.InputPCM() emits engine.PCMFrame; the
	// pipeline expects pipeline.Frame{Kind: KindPCM, PCM: ...}. We
	// wrap each frame and write into a dedicated channel that is
	// closed when the transport closes its side.
	source := make(chan pipeline.Frame, 32)
	go func() {
		defer close(source)
		in := port.InputPCM()
		for {
			select {
			case <-engCtx.Done():
				return
			case pcm, ok := <-in:
				if !ok {
					return
				}
				select {
				case source <- pipeline.Frame{Kind: pipeline.KindPCM, PCM: pcm}:
				case <-engCtx.Done():
					return
				}
			}
		}
	}()

	out, errs := pipe.Run(engCtx, source, lg)

	// PCM-out bridge: any KindPCM frame emerging from the pipeline
	// goes back to the caller via SendOutputPCM. The stubs don't
	// emit any (TTS is a stub), but the wiring is production-shaped.
	go func() {
		defer close(e.done)
		var stageErr error
		for {
			select {
			case <-engCtx.Done():
				stageErr = pipeline.Wait(errs)
				e.pipeErr = stageErr
				return
			case f, ok := <-out:
				if !ok {
					stageErr = pipeline.Wait(errs)
					e.pipeErr = stageErr
					return
				}
				if f.Kind != pipeline.KindPCM {
					continue
				}
				if sendErr := port.SendOutputPCM(f.PCM); sendErr != nil {
					lg.Warn("cascaded engine: SendOutputPCM failed; halting",
						engine.F("err", sendErr.Error()))
					cancel()
				}
			}
		}
	}()

	return e.detach, nil
}

// detach is the engine.Detach handle returned by Attach. Idempotent
// via sync.Once; safe to call from any goroutine.
//
// Behaviour:
//
//   - First call: cancels the engine context, waits for pipeline
//     drain, returns the joined stage error (or nil).
//   - Subsequent calls: instant no-op, return nil.
//   - ctx parameter bounds how long we wait for stages to drain;
//     when ctx fires before drain completes, returns ctx.Err() (the
//     pipeline error, if any, is still recorded internally and
//     surfaces on subsequent calls).
func (e *Engine) detach(ctx context.Context) error {
	var err error
	e.detachOnce.Do(func() {
		if e.cancel != nil {
			e.cancel()
		}
		if e.done == nil {
			return
		}
		select {
		case <-e.done:
			err = e.pipeErr
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}
