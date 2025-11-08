'use client';

import { useState, useMemo, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, ChevronDown, ChevronRight, Pin, Minimize2, MessageSquare } from 'lucide-react';
import { chatlogAPI, type Session } from '@/libs/ChatlogAPI';
import { cn } from '@/libs/utils';

interface GroupedSessions {
  pinned: Session[];
  normal: Session[];
  minimized: Session[];
}

const STORAGE_KEY_PINNED_COLLAPSED = 'chatlog_pinned_collapsed';
const STORAGE_KEY_MINIMIZED_EXPANDED = 'chatlog_minimized_expanded';

export function EnhancedSessionList() {
  const [pinnedCollapsed, setPinnedCollapsed] = useState<boolean>(() => {
    if (typeof window === 'undefined') return false;
    const saved = localStorage.getItem(STORAGE_KEY_PINNED_COLLAPSED);
    return saved === 'true';
  });

  const [minimizedExpanded, setMinimizedExpanded] = useState<boolean>(() => {
    if (typeof window === 'undefined') return false;
    const saved = localStorage.getItem(STORAGE_KEY_MINIMIZED_EXPANDED);
    return saved === 'true';
  });

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['sessions'],
    queryFn: () => chatlogAPI.getSessions({ format: 'json', limit: 100 }),
  });

  // Persist collapse/expand state
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_PINNED_COLLAPSED, String(pinnedCollapsed));
  }, [pinnedCollapsed]);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_MINIMIZED_EXPANDED, String(minimizedExpanded));
  }, [minimizedExpanded]);

  // Group sessions by type
  const groupedSessions = useMemo<GroupedSessions>(() => {
    if (!data?.items) {
      return { pinned: [], normal: [], minimized: [] };
    }

    return data.items.reduce<GroupedSessions>(
      (acc, session) => {
        if (session.isTopPinned) {
          acc.pinned.push(session);
        } else if (session.isHidden) {
          acc.minimized.push(session);
        } else {
          acc.normal.push(session);
        }
        return acc;
      },
      { pinned: [], normal: [], minimized: [] }
    );
  }, [data]);

  const togglePinnedCollapse = () => setPinnedCollapsed((prev) => !prev);
  const toggleMinimizedExpand = () => setMinimizedExpanded((prev) => !prev);

  const renderSession = (session: Session, showPin = false) => (
    <div
      key={session.userName}
      className={cn(
        'p-4 border rounded-lg transition-colors',
        'hover:bg-accent hover:border-accent-foreground/20'
      )}
    >
      <div className="flex justify-between items-start mb-2">
        <div className="flex items-center gap-2 flex-1">
          {session.avatarUrl && (
            <img
              src={session.avatarUrl}
              alt={session.nickName || session.userName}
              className="w-10 h-10 rounded-full object-cover"
            />
          )}
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="font-medium truncate">
                {session.nickName || session.userName}
              </span>
              {showPin && (
                <Pin className="h-3 w-3 text-primary flex-shrink-0" />
              )}
            </div>
            <div className="text-xs text-muted-foreground">{session.nTime}</div>
          </div>
        </div>
        {session.nUnReadCount > 0 && (
          <Badge variant="destructive" className="ml-2 flex-shrink-0">
            {session.nUnReadCount}
          </Badge>
        )}
      </div>
      <div className="text-sm text-muted-foreground line-clamp-2">
        {session.content}
      </div>
    </div>
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-col space-y-4">
        <div>
          <p className="text-sm text-muted-foreground mb-4">
            查询最近会话列表，支持置顶、普通和最小化分组显示。
            <Badge variant="secondary" className="ml-2">
              GET /api/v1/session
            </Badge>
          </p>
        </div>

        <div className="flex gap-2">
          <Button onClick={() => refetch()} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            查询会话
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <p className="text-destructive">
              错误: {error instanceof Error ? error.message : '未知错误'}
            </p>
          </CardContent>
        </Card>
      )}

      {data && (
        <div className="space-y-4">
          {/* Pinned Sessions */}
          {groupedSessions.pinned.length > 0 && (
            <Card className="border-primary/30 bg-primary/5">
              <CardContent className="pt-6">
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-between mb-3 hover:bg-primary/10"
                  onClick={togglePinnedCollapse}
                >
                  <div className="flex items-center gap-2">
                    {pinnedCollapsed ? (
                      <ChevronRight className="h-4 w-4" />
                    ) : (
                      <ChevronDown className="h-4 w-4" />
                    )}
                    <Pin className="h-4 w-4 text-primary" />
                    <span className="font-semibold">置顶会话</span>
                    <Badge variant="secondary">{groupedSessions.pinned.length}</Badge>
                  </div>
                </Button>
                {!pinnedCollapsed && (
                  <div className="space-y-2">
                    {groupedSessions.pinned.map((session) => renderSession(session, true))}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {/* Normal Sessions */}
          {groupedSessions.normal.length > 0 && (
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-2 mb-3">
                  <MessageSquare className="h-4 w-4 text-muted-foreground" />
                  <span className="font-semibold">普通会话</span>
                  <Badge variant="secondary">{groupedSessions.normal.length}</Badge>
                </div>
                <div className="space-y-2">
                  {groupedSessions.normal.map((session) => renderSession(session))}
                </div>
              </CardContent>
            </Card>
          )}

          {/* Minimized Sessions */}
          {groupedSessions.minimized.length > 0 && (
            <Card className="border-muted">
              <CardContent className="pt-6">
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-between mb-3"
                  onClick={toggleMinimizedExpand}
                >
                  <div className="flex items-center gap-2">
                    {minimizedExpanded ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                    <Minimize2 className="h-4 w-4 text-muted-foreground" />
                    <span className="font-semibold text-muted-foreground">最小化会话</span>
                    <Badge variant="outline">{groupedSessions.minimized.length}</Badge>
                  </div>
                </Button>
                {minimizedExpanded && (
                  <div className="space-y-2">
                    {groupedSessions.minimized.map((session) => renderSession(session))}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {groupedSessions.pinned.length === 0 &&
            groupedSessions.normal.length === 0 &&
            groupedSessions.minimized.length === 0 && (
              <Card>
                <CardContent className="pt-6">
                  <p className="text-center text-muted-foreground py-8">暂无会话</p>
                </CardContent>
              </Card>
            )}
        </div>
      )}
    </div>
  );
}
