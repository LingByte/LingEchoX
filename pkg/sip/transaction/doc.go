// Package transaction provides SIP UDP transaction helpers layered under pkg/sip/stack.Endpoint.
//
// UAC (client):
//   - INVITE client: RunInviteClient, HandleResponse (wire from stack.Endpoint.OnSIPResponse), ACK helpers.
//
// UAS INVITE (server):
//   - RegisterPendingInviteServer when an INVITE arrives before the final; ClearPendingInviteServer or let
//     BeginInviteServer clear it after a final.
//   - HandleCancelRequest for CANCEL matching pending Call-ID + CSeq (sends 200 to CANCEL); TU still sends 487 (or similar) on INVITE.
//   - After sending a final on the wire: BeginInviteServer; duplicate INVITE: HandleInviteRequest; 2xx: HandleAck.
//
// UAS non-INVITE (OPTIONS, REGISTER, …):
//   - After sending a final: BeginNonInviteServer; duplicate request: HandleNonInviteRequest (Timer J window).
//
// Keys: InviteTransactionKey(branch, Call-ID); NonInviteServerKey(req) for non-INVITE map.
//
// Not implemented: full server transaction for every edge case, TCP/TLS, forked responses.
package transaction
