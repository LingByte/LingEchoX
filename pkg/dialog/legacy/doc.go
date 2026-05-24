// Package legacy is the bridge between the new engine.Engine interface
// and the existing CallSession-based attach functions living in
// pkg/sip/conversation (AttachVoicePipeline and friends).
//
// Why this package exists
// -----------------------
//
// The dialog refactor (see docs/refactor-architecture.md) targets a
// world where every call path goes through engine.New(cfg).Attach(...).
// Today the call paths invoke conversation.AttachVoicePipeline directly,
// and that function owns 2200+ lines of cascaded + realtime logic that
// cannot be moved in one PR without significant risk.
//
// This package therefore ships a *paper-thin adapter*:
//
//   - engine.Engine and engine.Factory are satisfied by a struct that
//     simply delegates to a function-value Attacher.
//   - The Attacher is wired at init() time by the SIP-side package; the
//     conversation package owns the actual Attacher implementation and
//     can keep using its existing helpers, CallSession, zap.Logger, etc.
//   - This package has zero SIP / conversation imports so it cannot
//     create cycles with pkg/sip/* or pkg/dialog/engine consumers.
//
// What it explicitly does NOT do
// ------------------------------
//
//   - It does not move any code out of conversation. PR-3+ will start
//     extracting providers; PR-5+ will reimplement the engines natively
//     on the engine interface and this whole package will be deleted.
//   - It does not transform PCM through MediaPort. The legacy engines
//     attach directly to *sipSession.CallSession; the MediaPort handed
//     to Attach is forwarded as an opaque value through the LegacyHandle
//     escape hatch (type-asserted by the wired Attacher).
//
// Lifecycle expectation: temporary. This package SHOULD be removed in
// phase 6 of the refactor; new code MUST NOT depend on its types.
//
// Concurrency: Register* helpers are call-once at init time and panic
// on misuse. Attach is safe to call from the SIP attach goroutine.
package legacy
