// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Package tenantcfg owns the tenant-scoped voice configuration shape
// and its lookup machinery — the bits that any future engine
// (cascaded, realtime, multimodal) needs to make per-call decisions
// without depending on the SIP layer.
//
// Why a separate package?
//
//   - Phase 3 native engines live in pkg/dialog/* and must read tenant
//     voice config. Until this package existed, that data was buried
//     inside pkg/sip/conversation, which would have created an import
//     cycle (engine ← conversation ← engine).
//   - The legacy attachers (AttachCascadedLegacy / AttachRealtimeLegacy)
//     still need the same data; type aliases in pkg/sip/conversation
//     keep their call sites unchanged.
//
// Boundary discipline:
//
//   - This package MUST NOT import pkg/sip/* (would re-introduce the
//     cycle). The Resolve helper takes a uint tenantID, not a
//     *CallSession — callers extract the ID at the SIP boundary.
//   - This package MUST NOT touch zap loggers, file I/O, or media
//     pipelines. Logging belongs to the caller; this package only
//     does parse + validate + return.
package tenantcfg
