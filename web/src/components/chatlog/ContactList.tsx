'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Loader2, User } from 'lucide-react';
import { chatlogAPI } from '@/libs/ChatlogAPI';

export function ContactList() {
  const [keyword, setKeyword] = useState('');
  const [searchKeyword, setSearchKeyword] = useState('');

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['contacts', searchKeyword],
    queryFn: () =>
      chatlogAPI.getContacts({
        keyword: searchKeyword || undefined,
        format: 'json',
      }),
    enabled: false,
  });

  const handleQuery = () => {
    setSearchKeyword(keyword);
    refetch();
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col space-y-4">
        <div>
          <p className="text-sm text-muted-foreground mb-4">
            查询联系人列表,可选择性地按关键词搜索。
            <Badge variant="secondary" className="ml-2">
              GET /api/v1/contact
            </Badge>
          </p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="contact-keyword">
            搜索联系人
            <span className="text-xs text-muted-foreground ml-2">(可选)</span>
          </Label>
          <Input
            id="contact-keyword"
            placeholder="输入关键词搜索联系人"
            value={keyword}
            onChange={e => setKeyword(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleQuery()}
          />
        </div>

        <div className="flex gap-2">
          <Button onClick={handleQuery} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            查询联系人
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
        <Card>
          <CardContent className="pt-6">
            <div className="space-y-2">
              {data.items && data.items.length > 0 ? (
                data.items.map((contact, index) => (
                  <div
                    key={index}
                    className="flex items-center gap-4 p-4 border rounded-lg hover:bg-accent transition-colors"
                  >
                    <Avatar>
                      <AvatarImage
                        src={contact.contactHeadImgUrl?.smallHeadImgUrl || contact.contactHeadImgUrl?.bigHeadImgUrl}
                        alt={contact.nickName || contact.userName}
                      />
                      <AvatarFallback>
                        <User className="h-4 w-4" />
                      </AvatarFallback>
                    </Avatar>
                    <div className="flex-1 min-w-0">
                      <div className="font-medium truncate">
                        {contact.remark || contact.nickName || contact.userName}
                      </div>
                      {contact.remark && contact.nickName && (
                        <div className="text-sm text-muted-foreground truncate">
                          {contact.nickName}
                        </div>
                      )}
                      {contact.alias && (
                        <div className="text-xs text-muted-foreground truncate">
                          微信号: {contact.alias}
                        </div>
                      )}
                    </div>
                  </div>
                ))
              ) : (
                <p className="text-center text-muted-foreground py-8">暂无联系人</p>
              )}
            </div>
            {data.total > 0 && (
              <div className="mt-4 text-sm text-muted-foreground text-center">
                共 {data.total} 位联系人
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
