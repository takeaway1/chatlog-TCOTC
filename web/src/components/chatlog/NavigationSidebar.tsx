'use client';

import { useAtom } from 'jotai';
import { MessageCircle, Users, UserCircle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { activeSectionAtom } from '@/stores/chatlogStore';

const navItems = [
  { id: 'chats' as const, icon: MessageCircle, label: '聊天' },
  { id: 'contacts' as const, icon: UserCircle, label: '联系人' },
  { id: 'groups' as const, icon: Users, label: '群聊' },
];

export function NavigationSidebar() {
  const [activeSection, setActiveSection] = useAtom(activeSectionAtom);

  return (
    <div className="w-16 lg:w-20 bg-secondary/50 border-r border-border flex flex-col items-center py-4 gap-2">
      {navItems.map((item) => {
        const Icon = item.icon;
        const isActive = activeSection === item.id;

        return (
          <button
            key={item.id}
            onClick={() => setActiveSection(item.id)}
            className={cn(
              'flex flex-col items-center justify-center gap-1 w-12 h-12 lg:w-14 lg:h-14 rounded-lg transition-all',
              'hover:bg-background/80',
              isActive && 'bg-background shadow-sm text-primary'
            )}
            title={item.label}
          >
            <Icon className={cn('w-5 h-5 lg:w-6 lg:h-6', isActive && 'text-primary')} />
            <span className="text-xs hidden lg:block">{item.label}</span>
          </button>
        );
      })}
    </div>
  );
}
