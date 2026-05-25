// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

// persistStage observes turn boundaries and reports completed turns
// to a TurnPersister. It is a transparent pass-through stage placed
// at the tail of the cascaded pipeline so it sees every frame the
// downstream MediaPort consumer would see, but mutates none of them.
//
// What "a completed turn" means in our pipeline
// =============================================
//
//   KindTextFinal   — user-side ASR commit. Marks the start of a new
//                     turn record (accumulator reset). The Text
//                     field is the LLM input.
//   KindAIText      — one or more assistant-side delta fragments
//                     produced by llmStage. We concatenate them
//                     into the AIText field and record the
//                     wall-time of the first one for LLMFirstMs.
//   KindAITextDone  — assistant-side end-of-turn marker. We flush
//                     the accumulator into a TurnRecord and call
//                     PersistTurn. The TurnRecord is fire-and-forget
//                     from the stage's perspective — the persister
//                     is responsible for any background dispatch.
//
// Concurrency
// ===========
//
//   - persistStage owns its accumulator strictly inside Run; no
//     external concurrent access. Frames flow through one channel.
//   - PersistTurn is invoked from the Run goroutine; the persister
//     SHOULD return quickly (e.g. push to a buffered chan or fire a
//     goroutine internally) so it doesn't stall the pipeline.

import (
	"context"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// TurnRecord is the minimal payload the persistStage hands to the
// persister. SIP-side adapters translate it into the heavier
// DialogTurn / DB row shape; cascaded itself stays storage-neutral.
type TurnRecord struct {
	// UserText is the ASR-final transcript that triggered the turn.
	UserText string
	// AIText is the concatenation of every KindAIText delta in the
	// turn (LLM-side, NOT post-TTS-normalised).
	AIText string
	// LLMFirstMs is the wall-time from KindTextFinal to the first
	// KindAIText delta. Zero when no delta arrived (e.g. provider
	// error before any token).
	LLMFirstMs int
	// LLMWallMs is the wall-time from KindTextFinal to
	// KindAITextDone. Approximates QueryStream's duration; this
	// includes any TTS-side back-pressure if it stalls the
	// upstream stream.
	LLMWallMs int
	// PipelineMs equals LLMWallMs in the current Stage topology;
	// kept as a separate field because the legacy callers persist
	// both for analytics. Future TTS-aware stages can populate
	// it from the actual end-of-audio timestamp.
	PipelineMs int
	// CompletedAt is the wall-clock time KindAITextDone arrived.
	CompletedAt time.Time
}

// TurnPersister sinks completed turns. nil is treated as a no-op
// stage (passthrough only). Implementations MUST NOT block the
// caller; defer DB IO to a goroutine or buffered channel inside.
type TurnPersister interface {
	PersistTurn(ctx context.Context, rec TurnRecord)
}

// TurnPersisterFunc adapts a plain function into a TurnPersister.
type TurnPersisterFunc func(ctx context.Context, rec TurnRecord)

// PersistTurn implements TurnPersister.
func (f TurnPersisterFunc) PersistTurn(ctx context.Context, rec TurnRecord) {
	if f != nil {
		f(ctx, rec)
	}
}

// persistStage is the pipeline.Stage that builds TurnRecords from
// the frame stream.
type persistStage struct {
	persister TurnPersister
	nowFn     func() time.Time // injectable for tests
}

func newPersistStage(p TurnPersister) *persistStage {
	return &persistStage{persister: p, nowFn: time.Now}
}

// Name implements pipeline.Stage.
func (persistStage) Name() string { return "persist" }

// Run implements pipeline.Stage.
//
//   in  → KindTextFinal     → start accumulator
//   in  → KindAIText        → append to AIText, mark LLMFirstMs
//   in  → KindAITextDone    → flush TurnRecord
//   in  → any other frame   → passthrough
//   in  closed              → flush any in-flight accumulator then exit
//
// Always passthrough so downstream consumers (TTS, MediaPort) see
// every frame regardless of persistence wiring.
func (s *persistStage) Run(
	ctx context.Context,
	in <-chan pipeline.Frame,
	out chan<- pipeline.Frame,
	lg engine.Logger,
) error {
	defer close(out)

	type acc struct {
		userText  string
		aiText    strings.Builder
		startedAt time.Time
		firstAIAt time.Time
		active    bool
	}
	var a acc

	flush := func(completedAt time.Time) {
		if !a.active || s.persister == nil {
			a = acc{}
			return
		}
		rec := TurnRecord{
			UserText:    a.userText,
			AIText:      strings.TrimSpace(a.aiText.String()),
			CompletedAt: completedAt,
		}
		if !a.firstAIAt.IsZero() && !a.startedAt.IsZero() {
			rec.LLMFirstMs = int(a.firstAIAt.Sub(a.startedAt).Milliseconds())
		}
		if !completedAt.IsZero() && !a.startedAt.IsZero() {
			rec.LLMWallMs = int(completedAt.Sub(a.startedAt).Milliseconds())
			rec.PipelineMs = rec.LLMWallMs
		}
		s.persister.PersistTurn(ctx, rec)
		a = acc{}
	}

	for {
		select {
		case <-ctx.Done():
			// Don't flush on ctx cancel — the turn was interrupted,
			// not completed. Persisters that want abort-records can
			// register their own ctx observer.
			return ctx.Err()
		case f, ok := <-in:
			if !ok {
				// in closed → drain incomplete turn if anything
				// meaningful was accumulated.
				if a.active && a.aiText.Len() > 0 {
					flush(s.nowFn())
				}
				return nil
			}
			switch f.Kind {
			case pipeline.KindTextFinal:
				// New turn boundary: flush any stale accumulator
				// (defensive — should already be flushed via
				// KindAITextDone) and start fresh.
				if a.active {
					flush(s.nowFn())
				}
				a = acc{
					userText:  strings.TrimSpace(f.Text),
					startedAt: s.nowFn(),
					active:    true,
				}
			case pipeline.KindAIText:
				if !a.active {
					// AI delta without a preceding user-final —
					// can happen on welcome/auto-prompt paths.
					// Start an accumulator with empty userText so
					// the assistant utterance still gets recorded.
					a = acc{
						startedAt: s.nowFn(),
						active:    true,
					}
				}
				if a.firstAIAt.IsZero() {
					a.firstAIAt = s.nowFn()
				}
				a.aiText.WriteString(f.Text)
			case pipeline.KindAITextDone:
				flush(s.nowFn())
			}
			if err := sendOrCancel(ctx, out, f); err != nil {
				return err
			}
		}
	}
}
