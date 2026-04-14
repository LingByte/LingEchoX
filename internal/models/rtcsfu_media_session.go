// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package models

import "time"

// RTCSFUMediaSession records a participant WebRTC leg connected to the embedded SFU.
type RTCSFUMediaSession struct {
	BaseModel
	RoomID    string     `json:"roomId" gorm:"size:160;index:idx_rtcsfu_media_room_peer,priority:1;not null"`
	PeerID    string     `json:"peerId" gorm:"size:160;index:idx_rtcsfu_media_room_peer,priority:2;not null"`
	RemoteIP  string     `json:"remoteIp" gorm:"size:64"`
	UserAgent string     `json:"userAgent" gorm:"size:512"`
	JoinedAt  time.Time  `json:"joinedAt" gorm:"comment:WS connect time"`
	LeftAt    *time.Time `json:"leftAt,omitempty" gorm:"comment:Disconnect time"`
}

func (RTCSFUMediaSession) TableName() string {
	return "rtcsfu_media_sessions"
}
