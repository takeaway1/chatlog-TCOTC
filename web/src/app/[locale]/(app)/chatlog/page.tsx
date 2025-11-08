import type { Metadata } from 'next';
import { ChatlogDashboard } from '@/components/chatlog/ChatlogDashboard';

export const metadata: Metadata = {
  title: 'Chatlog - 聊天记录查看器',
  description: '查看和管理你的微信聊天记录',
};

export default function ChatlogPage() {
  return <ChatlogDashboard />;
}
