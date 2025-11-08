'use client';

import { useAtom } from 'jotai';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Loader2, MessageSquare, Download, AlertCircle, Image as ImageIcon, User } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import { chatlogAPI, type GetChatlogParams } from '@/libs/ChatlogAPI';
import { format } from 'date-fns';
import { ExportDialog } from './ExportDialog';
import {
  chatlogParamsAtom,
  chatlogSearchParamsAtom,
  chatlogValidationErrorAtom,
  exportDialogOpenAtom,
} from '@/stores/chatlogStore';

export function ChatlogViewer() {
  const [params, setParams] = useAtom(chatlogParamsAtom);
  const [searchParams, setSearchParams] = useAtom(chatlogSearchParamsAtom);
  const [exportDialogOpen, setExportDialogOpen] = useAtom(exportDialogOpenAtom);
  const [validationError, setValidationError] = useAtom(chatlogValidationErrorAtom);
  const [previewImage, setPreviewImage] = useState<string | null>(null);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['chatlog', searchParams],
    queryFn: () => chatlogAPI.getChatlog({ ...searchParams!, format: 'json' }),
    enabled: searchParams !== null,
  });

  const handleQuery = () => {
    setValidationError('');

    if (!params.time || params.time.trim() === '') {
      setValidationError('时间范围是必填项');
      return;
    }

    const cleanParams: GetChatlogParams = {
      time: params.time,
    };

    if (params.talker) cleanParams.talker = params.talker;
    if (params.sender) cleanParams.sender = params.sender;
    if (params.keyword) cleanParams.keyword = params.keyword;
    if (params.limit) cleanParams.limit = params.limit;

    setSearchParams(cleanParams);
  };

  const formatTime = (timestamp: number) => {
    try {
      return format(new Date(timestamp * 1000), 'yyyy-MM-dd HH:mm:ss');
    }
    catch {
      return String(timestamp);
    }
  };

  const getImageUrl = (message: any): string | null => {
    if (message.type !== 3 || !message.contents) {
      return null;
    }

    // Priority: md5 > imgfile
    if (message.contents.md5) {
      return chatlogAPI.getImageURL(message.contents.md5);
    }

    if (message.contents.imgfile) {
      return chatlogAPI.getMediaDataURL(message.contents.imgfile);
    }

    return null;
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col space-y-4">
        <div>
          <p className="text-sm text-muted-foreground mb-4">
            查询聊天记录，支持多种筛选条件。
            <Badge variant="secondary" className="ml-2">
              GET /api/v1/chatlog
            </Badge>
          </p>
        </div>

        {validationError && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{validationError}</AlertDescription>
          </Alert>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="chatlog-time">
              时间范围
              <span className="text-xs text-destructive ml-2">*必填</span>
            </Label>
            <Input
              id="chatlog-time"
              placeholder="例如: last-7d, today, 2024-01-01~2024-01-31"
              value={params.time}
              onChange={e => setParams({ ...params, time: e.target.value })}
              required
            />
            <p className="text-xs text-muted-foreground">
              支持: last-7d, today, yesterday, 2024-01-01, 2024-01-01~2024-01-31 等
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="chatlog-talker">
              聊天对象
              <span className="text-xs text-muted-foreground ml-2">(可选)</span>
            </Label>
            <Input
              id="chatlog-talker"
              placeholder="微信ID、备注名或昵称"
              value={params.talker}
              onChange={e => setParams({ ...params, talker: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="chatlog-sender">
              发送者
              <span className="text-xs text-muted-foreground ml-2">(可选)</span>
            </Label>
            <Input
              id="chatlog-sender"
              placeholder="发送者的微信ID"
              value={params.sender}
              onChange={e => setParams({ ...params, sender: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="chatlog-keyword">
              关键词
              <span className="text-xs text-muted-foreground ml-2">(可选)</span>
            </Label>
            <Input
              id="chatlog-keyword"
              placeholder="搜索消息内容"
              value={params.keyword}
              onChange={e => setParams({ ...params, keyword: e.target.value })}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="chatlog-limit">返回记录数</Label>
            <Input
              id="chatlog-limit"
              type="number"
              placeholder="默认100"
              value={params.limit || ''}
              onChange={e =>
                setParams({ ...params, limit: Number.parseInt(e.target.value) || undefined })
              }
            />
          </div>
        </div>

        <div className="flex gap-2">
          <Button onClick={handleQuery} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            查询聊天记录
          </Button>
          {data && data.length > 0 && (
            <Button
              variant="secondary"
              onClick={() => setExportDialogOpen(true)}
            >
              <Download className="mr-2 h-4 w-4" />
              导出 ({data.length})
            </Button>
          )}
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
              {data && data.length > 0 ? (
                data.map((message, index) => {
                  const isSystemMsg = message.type === 10000;
                  const isImageMsg = message.type === 3;
                  const isReferMsg = message.type === 49 && message.subType === 57;

                  if (isSystemMsg) {
                    return (
                      <div key={index} className="flex justify-center py-1">
                        <div className="text-xs text-muted-foreground bg-muted/50 px-3 py-1 rounded">
                          系统消息
                        </div>
                      </div>
                    );
                  }

                  return (
                    <div
                      key={index}
                      className={`flex gap-2 ${message.isSender ? 'flex-row-reverse' : 'flex-row'}`}
                    >
                      {/* Avatar */}
                      <div className="flex-shrink-0 mt-1">
                        <Avatar className="w-10 h-10">
                          <AvatarImage
                            src={message.senderAvatar}
                            alt={message.senderName || message.sender || '用户'}
                          />
                          <AvatarFallback className={message.isSender ? 'bg-primary text-primary-foreground' : 'bg-muted'}>
                            {message.isSender
                              ? '我'
                              : (message.senderName || message.sender || '?').slice(0, 2).toUpperCase()
                            }
                          </AvatarFallback>
                        </Avatar>
                      </div>

                      {/* Message bubble */}
                      <div className={`flex flex-col max-w-[70%] ${message.isSender ? 'items-end' : 'items-start'}`}>
                        {/* Sender name and time */}
                        <div className={`flex gap-2 items-center mb-1 px-1 ${message.isSender ? 'flex-row-reverse' : 'flex-row'}`}>
                          <span className="text-xs text-muted-foreground font-medium">
                            {message.isSender ? '我' : (message.senderName || message.sender || message.talker)}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {formatTime(message.createTime)}
                          </span>
                        </div>

                        {/* Message content */}
                        <div
                          className={`rounded-lg ${
                            isImageMsg ? 'p-1' : 'px-3 py-2'
                          } ${
                            message.isSender
                              ? 'bg-primary text-primary-foreground'
                              : 'bg-muted'
                          }`}
                        >
                          {isImageMsg && (() => {
                            const imageUrl = getImageUrl(message);
                            return imageUrl ? (
                              <div className="relative group">
                                <img
                                  src={imageUrl}
                                  alt="聊天图片"
                                  className="max-w-[240px] max-h-[320px] rounded cursor-pointer object-cover"
                                  onClick={() => setPreviewImage(imageUrl)}
                                  onError={(e) => {
                                    e.currentTarget.style.display = 'none';
                                    e.currentTarget.nextElementSibling?.classList.remove('hidden');
                                  }}
                                />
                                <div className="hidden flex items-center gap-2 text-sm p-2">
                                  <ImageIcon className="h-4 w-4" />
                                  <span>[图片加载失败]</span>
                                </div>
                              </div>
                            ) : (
                              <div className="flex items-center gap-2 text-sm p-2">
                                <ImageIcon className="h-4 w-4" />
                                <span>[图片消息]</span>
                              </div>
                            );
                          })()}

                          {isReferMsg && (
                            <div className="mb-2 p-2 rounded bg-black/10 border-l-2 border-current text-xs opacity-75">
                              引用消息
                            </div>
                          )}

                          {!isImageMsg && (
                            <div className="text-sm whitespace-pre-wrap break-words">
                              {message.displayContent || message.content || '[无内容]'}
                            </div>
                          )}

                          {message.type !== 1 && message.type !== 3 && message.type !== 49 && (
                            <Badge variant="outline" className="mt-1 text-xs">
                              类型 {message.type}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </div>
                  );
                })
              ) : (
                <div className="text-center text-muted-foreground py-12">
                  <MessageSquare className="h-12 w-12 mx-auto mb-3 opacity-20" />
                  <p>暂无聊天记录</p>
                </div>
              )}
            </div>
            {data && data.length > 0 && (
              <div className="mt-6 pt-4 border-t text-sm text-muted-foreground text-center">
                共 {data.length} 条消息
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {data && data.length > 0 && (
        <ExportDialog
          open={exportDialogOpen}
          onOpenChange={setExportDialogOpen}
          messages={data}
        />
      )}

      {/* Image Preview Dialog */}
      <Dialog open={!!previewImage} onOpenChange={(open) => !open && setPreviewImage(null)}>
        <DialogContent className="max-w-4xl max-h-[90vh] p-2">
          {previewImage && (
            <div className="flex items-center justify-center">
              <img
                src={previewImage}
                alt="预览"
                className="max-w-full max-h-[85vh] object-contain rounded"
              />
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
