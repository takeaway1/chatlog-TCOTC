package model

import "time"

// 注意，v4 session 是独立数据库文件
// CREATE TABLE SessionTable(
// username TEXT PRIMARY KEY,
// type INTEGER,
// unread_count INTEGER,
// unread_first_msg_srv_id INTEGER,
// is_hidden INTEGER,
// summary TEXT,
// draft TEXT,
// status INTEGER,
// last_timestamp INTEGER,
// sort_timestamp INTEGER,
// last_clear_unread_timestamp INTEGER,
// last_msg_locald_id INTEGER,
// last_msg_type INTEGER,
// last_msg_sub_type INTEGER,
// last_msg_sender TEXT,
// last_sender_display_name TEXT,
// last_msg_ext_type INTEGER
// )
type SessionV4 struct {
	Username              string `json:"username"`
	Summary               string `json:"summary"`
	LastTimestamp         int    `json:"last_timestamp"`
	LastMsgSender         string `json:"last_msg_sender"`
	LastSenderDisplayName string `json:"last_sender_display_name"`
	UnreadCount           int    `json:"unread_count"`
	IsHidden              int    `json:"is_hidden"`
	SortTimestamp         int64  `json:"sort_timestamp"`

	// Type                     int    `json:"type"`
	// UnreadFirstMsgSrvID      int    `json:"unread_first_msg_srv_id"`
	// Draft                    string `json:"draft"`
	// Status                   int    `json:"status"`
	// LastClearUnreadTimestamp int    `json:"last_clear_unread_timestamp"`
	// LastMsgLocaldID          int    `json:"last_msg_locald_id"`
	// LastMsgType              int    `json:"last_msg_type"`
	// LastMsgSubType           int    `json:"last_msg_sub_type"`
	// LastMsgExtType           int    `json:"last_msg_ext_type"`
}

func (s *SessionV4) Wrap() *Session {
	// Determine if session is pinned: sort_timestamp significantly larger than last_timestamp
	// Typically, pinned sessions have sort_timestamp set to a very large value (e.g., far in the future)
	const pinnedThreshold = 86400 * 365 * 10 // 10 years in seconds
	isTopPinned := s.SortTimestamp > int64(s.LastTimestamp)+pinnedThreshold

	return &Session{
		UserName:     s.Username,
		NOrder:       s.LastTimestamp,
		NickName:     s.LastSenderDisplayName,
		Content:      s.Summary,
		NTime:        time.Unix(int64(s.LastTimestamp), 0),
		NUnReadCount: s.UnreadCount,
		ParentRef:    "",
		IsTopPinned:  isTopPinned,
		IsHidden:     s.IsHidden == 1,
		SortOrder:    s.SortTimestamp,
	}
}
