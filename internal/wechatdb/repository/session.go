package repository

import (
	"context"
	"sort"
	"strings"

	"github.com/sjzar/chatlog/internal/model"
)

func (r *Repository) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	// Step 1: Get pinned usernames from contact table
	pinnedUserNames, err := r.ds.GetPinnedUserNames(ctx)
	if err != nil {
		return nil, err
	}
	pinnedMap := make(map[string]bool, len(pinnedUserNames))
	for _, userName := range pinnedUserNames {
		pinnedMap[userName] = true
	}

	// Step 2: Collect all sessions (pinned + regular)
	sessionMap := make(map[string]*model.Session)

	// First, load pinned sessions individually to ensure they're included
	for _, userName := range pinnedUserNames {
		sessions, err := r.ds.GetSessions(ctx, userName, 1, 0)
		if err != nil {
			continue // Skip if error
		}
		if len(sessions) > 0 {
			sessionMap[userName] = sessions[0]
		}
	}

	// Then load regular sessions (load more to ensure we have enough after filtering)
	fetchLimit := limit * 2
	if fetchLimit < 50 {
		fetchLimit = 50 // Minimum fetch size
	}
	sessions, err := r.ds.GetSessions(ctx, key, fetchLimit, 0)
	if err != nil {
		return nil, err
	}

	// Add regular sessions to map (won't overwrite pinned sessions already in map)
	for _, session := range sessions {
		if _, exists := sessionMap[session.UserName]; !exists {
			sessionMap[session.UserName] = session
		}
	}

	// Step 3: Enrich sessions with contact/chatroom information and filter placeholders
	result := make([]*model.Session, 0, len(sessionMap))
	for _, session := range sessionMap {
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

			// Set pinned and minimized status from contact
			session.IsPinned = contact.IsPinned
			session.IsMinimized = contact.IsMinimized
		} else {
			// Try to get chatroom/group contact info directly from contact table
			// (GetContacts excludes chatrooms, so we query directly here)
			if strings.Contains(session.UserName, "@chatroom") {
				contacts, err := r.ds.GetContacts(ctx, session.UserName, 1, 0)
				if err == nil && len(contacts) > 0 {
					contact := contacts[0]
					session.NickName = contact.DisplayName()
					if contact.SmallHeadImgUrl != "" {
						session.AvatarURL = contact.SmallHeadImgUrl
					} else if contact.BigHeadImgUrl != "" {
						session.AvatarURL = contact.BigHeadImgUrl
					}
					session.IsPinned = contact.IsPinned
					session.IsMinimized = contact.IsMinimized
				}
			}

			// Fallback: try ChatRoom table
			if session.NickName == "" {
				if chatroom, err := r.GetChatRoom(ctx, session.UserName); err == nil && chatroom != nil {
					session.NickName = chatroom.DisplayName()
				}
			}

			// Set pinned status from pinnedMap if not already set
			if pinnedMap[session.UserName] {
				session.IsPinned = true
			}
		}

		result = append(result, session)
	}

	// Step 4: Sort by pinned status first, then by time
	// Priority: 1. Pinned (non-minimized, non-public) 2. Regular (non-minimized, non-public) 3. Others
	sort.Slice(result, func(i, j int) bool {
		// Helper: check if session is a public account
		isPublicI := strings.HasPrefix(result[i].UserName, "gh_")
		isPublicJ := strings.HasPrefix(result[j].UserName, "gh_")

		// Helper: check if session should be in main list (not minimized and not public)
		isMainI := !result[i].IsMinimized && !isPublicI
		isMainJ := !result[j].IsMinimized && !isPublicJ

		// Both in main list: pinned first, then by time
		if isMainI && isMainJ {
			if result[i].IsPinned != result[j].IsPinned {
				return result[i].IsPinned
			}
			return result[i].NTime.After(result[j].NTime)
		}

		// Main list items come before non-main items
		if isMainI != isMainJ {
			return isMainI
		}

		// Both not in main list: sort by time
		return result[i].NTime.After(result[j].NTime)
	})

	// Step 5: Apply pagination
	start := offset
	end := offset + limit
	if start > len(result) {
		return []*model.Session{}, nil
	}
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], nil
}
