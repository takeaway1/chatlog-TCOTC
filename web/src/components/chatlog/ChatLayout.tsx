'use client';

import { NavigationSidebar } from './NavigationSidebar';
import { ConversationListPanel } from './ConversationListPanel';
import { ChatPanel } from './ChatPanel';

export function ChatLayout() {
  return (
    <div className="h-screen flex overflow-hidden bg-background">
      {/* Left: Navigation Sidebar */}
      <NavigationSidebar />

      {/* Middle: Conversation List */}
      <ConversationListPanel />

      {/* Right: Chat Panel */}
      <ChatPanel />
    </div>
  );
}
