'use client';

import { useAtom } from 'jotai';
import { MessageCircle } from 'lucide-react';
import { NavigationSidebar } from './NavigationSidebar';
import { ConversationListPanel } from './ConversationListPanel';
import { ChatPanel } from './ChatPanel';
import { selectedConversationAtom } from '@/stores/chatlogStore';

export function ChatLayout() {
  const [selectedConversation] = useAtom(selectedConversationAtom);

  return (
    <div className="min-h-screen bg-secondary/20 flex flex-col">
      {/* Header */}
      <header className="bg-background border-b border-border">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
              <MessageCircle className="w-5 h-5 text-primary-foreground" />
            </div>
            <div>
              <h1 className="text-lg font-semibold">Lovchat</h1>
              <p className="text-xs text-muted-foreground">微信聊天记录查看器</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <div className="hidden sm:flex items-center gap-2 text-xs text-muted-foreground">
              <div className="w-2 h-2 rounded-full bg-green-500"></div>
              <span>服务运行中</span>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content - Centered Tablet Size */}
      <div className="flex-1 flex items-center justify-center p-4 lg:p-8">
        <div className="w-full max-w-6xl h-[calc(100vh-8rem)] lg:h-[calc(100vh-12rem)] flex overflow-hidden bg-background rounded-xl shadow-lg border border-border">
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
      </div>

      {/* Footer */}
      <footer className="bg-background border-t border-border">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-12 flex items-center justify-center">
          <p className="text-xs text-muted-foreground">
            所有数据仅存储在本地 • Lovchat © 2025
          </p>
        </div>
      </footer>
    </div>
  );
}
