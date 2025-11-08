'use client';

import { useAtom } from 'jotai';
import { NavigationSidebar } from './NavigationSidebar';
import { ConversationListPanel } from './ConversationListPanel';
import { ChatPanel } from './ChatPanel';
import { selectedConversationAtom } from '@/stores/chatlogStore';

export function ChatLayout() {
  const [selectedConversation] = useAtom(selectedConversationAtom);

  return (
    <div className="h-screen flex overflow-hidden bg-background">
      {/* Left: Navigation Sidebar - hidden on mobile when chat is open */}
      <div className={selectedConversation ? 'hidden lg:flex' : 'flex'}>
        <NavigationSidebar />
      </div>

      {/* Middle: Conversation List - hidden on mobile when chat is open */}
      <div className={selectedConversation ? 'hidden md:flex' : 'flex flex-1'}>
        <ConversationListPanel />
      </div>

      {/* Right: Chat Panel - hidden on mobile when no chat selected */}
      <div className={selectedConversation ? 'flex flex-1' : 'hidden md:flex md:flex-1'}>
        <ChatPanel />
      </div>
    </div>
  );
}
