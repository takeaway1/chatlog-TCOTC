package repository

import (
	"context"

	"github.com/sjzar/chatlog/internal/model"
)

func (r *Repository) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	sessions, err := r.ds.GetSessions(ctx, key, limit, offset)
	if err != nil {
		return nil, err
	}

	// Enrich sessions with contact/chatroom information and filter placeholders
	result := make([]*model.Session, 0, len(sessions))
	for _, session := range sessions {
		// Skip placeholder sessions
		if session.UserName == "@placeholder_foldgroup" || session.UserName == "" {
			continue
		}

		// Get contact or chatroom information for the userName
		if contact := r.findContact(session.UserName); contact != nil {
			// Use contact's display name, not the last message sender's name
			session.NickName = contact.DisplayName()

			// Set avatar
			if contact.SmallHeadImgUrl != "" {
				session.AvatarURL = contact.SmallHeadImgUrl
			} else if contact.BigHeadImgUrl != "" {
				session.AvatarURL = contact.BigHeadImgUrl
			}
		} else if chatroom, err := r.GetChatRoom(ctx, session.UserName); err == nil && chatroom != nil {
			// For chatrooms, use chatroom display name
			session.NickName = chatroom.DisplayName()
		}

		result = append(result, session)
	}

	return result, nil
}
