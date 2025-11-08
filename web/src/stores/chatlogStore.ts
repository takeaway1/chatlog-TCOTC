import { atom } from 'jotai';
import { atomWithStorage } from 'jotai/utils';
import type { GetChatlogParams, Message } from '@/libs/ChatlogAPI';

// Navigation section state (WeChat-style)
export const activeSectionAtom = atomWithStorage<'chats' | 'contacts' | 'groups'>(
  'chatlog_active_section',
  'chats',
);

// Selected conversation state
export type SelectedConversation = {
  type: 'session' | 'contact' | 'chatroom';
  id: string; // userName for sessions/contacts, chatroom name for groups
  displayName: string;
  avatar?: string;
} | null;

export const selectedConversationAtom = atom<SelectedConversation>(null);

// Messages for selected conversation
export const conversationMessagesAtom = atom<Message[]>([]);

// Legacy tab state (kept for backward compatibility)
export const activeTabAtom = atomWithStorage<'session' | 'chatroom' | 'contact' | 'chatlog'>(
  'chatlog_active_tab',
  'session',
);

// Chatlog query parameters with localStorage persistence
export const chatlogParamsAtom = atomWithStorage<GetChatlogParams>(
  'chatlog_query_params',
  {
    time: 'last-7d',
    talker: '',
    sender: '',
    keyword: '',
    limit: 100,
  },
);

// Search trigger state (to trigger query execution)
export const chatlogSearchParamsAtom = atom<GetChatlogParams | null>(null);

// Validation error state
export const chatlogValidationErrorAtom = atom<string>('');

// Export dialog state
export const exportDialogOpenAtom = atom<boolean>(false);
