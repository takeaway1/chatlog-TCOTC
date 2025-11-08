'use client';

import { useAtom } from 'jotai';
import { useQuery } from '@tanstack/react-query';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Loader2, Search } from 'lucide-react';
import { cn } from '@/lib/utils';
import { activeSectionAtom, selectedConversationAtom, type SelectedConversation } from '@/stores/chatlogStore';
import { chatlogAPI } from '@/libs/ChatlogAPI';
import { useState } from 'react';

export function ConversationListPanel() {
  const [activeSection] = useAtom(activeSectionAtom);
  const [selectedConversation, setSelectedConversation] = useAtom(selectedConversationAtom);
  const [searchKeyword, setSearchKeyword] = useState('');

  // Fetch sessions
  const { data: sessions, isLoading: isLoadingSessions, error: sessionsError } = useQuery({
    queryKey: ['sessions'],
    queryFn: () => chatlogAPI.getSessions({ format: 'json' }),
    enabled: activeSection === 'chats',
    staleTime: 1000 * 60 * 5, // Cache for 5 minutes
    gcTime: 1000 * 60 * 10, // Keep in cache for 10 minutes
  });

  // Fetch contacts
  const { data: contacts, isLoading: isLoadingContacts, error: contactsError } = useQuery({
    queryKey: ['contacts'],
    queryFn: () => chatlogAPI.getContacts({ format: 'json' }),
    enabled: activeSection === 'contacts',
    staleTime: 1000 * 60 * 30, // Cache for 30 minutes (contacts change less frequently)
    gcTime: 1000 * 60 * 60, // Keep in cache for 1 hour
  });

  // Fetch chatrooms
  const { data: chatrooms, isLoading: isLoadingChatrooms, error: chatroomsError } = useQuery({
    queryKey: ['chatrooms'],
    queryFn: () => chatlogAPI.getChatRooms({ format: 'json' }),
    enabled: activeSection === 'groups',
    staleTime: 1000 * 60 * 30, // Cache for 30 minutes
    gcTime: 1000 * 60 * 60, // Keep in cache for 1 hour
  });

  const isLoading = isLoadingSessions || isLoadingContacts || isLoadingChatrooms;
  const error = sessionsError || contactsError || chatroomsError;

  // Filter and map data based on active section
  const items = (() => {
    const keyword = searchKeyword.toLowerCase();

    if (activeSection === 'chats' && sessions?.items) {
      return sessions.items
        .filter(s =>
          !keyword ||
          s.userName.toLowerCase().includes(keyword) ||
          s.nickName?.toLowerCase().includes(keyword) ||
          s.content?.toLowerCase().includes(keyword)
        )
        .map(session => ({
          type: 'session' as const,
          id: session.userName,
          displayName: session.nickName || session.userName,
          avatar: undefined,
          subtitle: session.content,
          time: session.nTime,
          unreadCount: session.nUnReadCount,
        }));
    }

    if (activeSection === 'contacts' && contacts?.items) {
      return contacts.items
        .filter(c =>
          !keyword ||
          c.userName.toLowerCase().includes(keyword) ||
          c.nickName?.toLowerCase().includes(keyword) ||
          c.remark?.toLowerCase().includes(keyword)
        )
        .map(contact => ({
          type: 'contact' as const,
          id: contact.userName,
          displayName: contact.remark || contact.nickName || contact.userName,
          avatar: contact.contactHeadImgUrl?.smallHeadImgUrl,
          subtitle: contact.userName,
        }));
    }

    if (activeSection === 'groups' && chatrooms?.items) {
      return chatrooms.items
        .filter(g =>
          !keyword ||
          g.name.toLowerCase().includes(keyword) ||
          g.nickName?.toLowerCase().includes(keyword)
        )
        .map(group => ({
          type: 'chatroom' as const,
          id: group.name,
          displayName: group.nickName || group.name,
          avatar: undefined,
          subtitle: `${group.users?.length || 0} 成员`,
        }));
    }

    return [];
  })();

  const handleSelectItem = (item: typeof items[0]) => {
    const conversation: SelectedConversation = {
      type: item.type,
      id: item.id,
      displayName: item.displayName,
      avatar: item.avatar,
    };
    setSelectedConversation(conversation);
  };

  return (
    <div className="w-full lg:w-80 bg-background border-r border-border flex flex-col">
      {/* Search bar */}
      <div className="p-4 border-b border-border">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="搜索"
            value={searchKeyword}
            onChange={(e) => setSearchKeyword(e.target.value)}
            className="pl-9"
          />
        </div>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto">
        {error ? (
          <div className="flex flex-col items-center justify-center py-12 px-4">
            <p className="text-sm text-destructive text-center mb-2">加载失败</p>
            <p className="text-xs text-muted-foreground text-center">
              {error instanceof Error ? error.message : '未知错误'}
            </p>
          </div>
        ) : isLoading ? (
          <div className="flex flex-col items-center justify-center py-12">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground mb-2" />
            <p className="text-xs text-muted-foreground">加载中...</p>
          </div>
        ) : items.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">
              {searchKeyword ? '无搜索结果' : '暂无数据'}
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {items.map((item) => {
              const isSelected = selectedConversation?.id === item.id && selectedConversation?.type === item.type;

              return (
                <button
                  key={`${item.type}-${item.id}`}
                  onClick={() => handleSelectItem(item)}
                  className={cn(
                    'w-full p-4 flex items-start gap-3 hover:bg-accent/50 transition-colors text-left',
                    isSelected && 'bg-accent'
                  )}
                >
                  <Avatar className="w-12 h-12 flex-shrink-0">
                    <AvatarImage src={item.avatar} alt={item.displayName} />
                    <AvatarFallback className="bg-primary/10 text-primary">
                      {item.displayName.slice(0, 2).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-baseline justify-between gap-2 mb-1">
                      <span className="font-medium text-sm truncate">
                        {item.displayName}
                      </span>
                      {'time' in item && item.time && (
                        <span className="text-xs text-muted-foreground flex-shrink-0">
                          {item.time}
                        </span>
                      )}
                    </div>

                    <div className="flex items-center justify-between gap-2">
                      <p className="text-sm text-muted-foreground truncate flex-1">
                        {item.subtitle}
                      </p>
                      {'unreadCount' in item && item.unreadCount > 0 && (
                        <Badge variant="destructive" className="flex-shrink-0 h-5 min-w-5 px-1.5 text-xs">
                          {item.unreadCount > 99 ? '99+' : item.unreadCount}
                        </Badge>
                      )}
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
