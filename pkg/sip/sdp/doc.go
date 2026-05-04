// Package sdp parses and generates minimal audio SDP bodies for SIP/VoIP.
//
// It supports common RTP/AVP (and RTP/AVPF) audio offers/answers with a=rtpmap lines,
// static payload types (0/8/9) when rtpmap is omitted, and rejects SRTP-only media lines (SAVP/SAVPF).
//
// [NormalizeBody] trims and normalizes CRLF/LF before [Parse] so offers from heterogeneous UAs parse consistently.
//
// Production SIP stacks import this package directly for Parse / Generate / codec negotiate helpers.
//
// After you map SDP codecs to names (pcmu, pcma, opus, g722), use pkg/media/encoder.CreateDecode /
// CreateEncode — do not reimplement those codecs in sip1.
package sdp
