'use client';

import { useState, useEffect, useMemo } from 'react';
import { Download, FileText, Copy, Check } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Separator } from '@/components/ui/separator';
import type { Message } from '@/libs/ChatlogAPI';
import {
  type ExportFormat,
  downloadExport,
  generateExportContent,
  getFormatLabel,
  getFormatDescription,
} from '@/utils/exportChatlog';

interface ExportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  messages: Message[];
}

interface SenderRename {
  [original: string]: string;
}

export function ExportDialog({ open, onOpenChange, messages }: ExportDialogProps) {
  const [selectedFormat, setSelectedFormat] = useState<ExportFormat>('interview');
  const [previewContent, setPreviewContent] = useState<string>('');
  const [filterSystemMessages, setFilterSystemMessages] = useState(false);
  const [senderRenames, setSenderRenames] = useState<SenderRename>({});
  const [copied, setCopied] = useState(false);

  const formats: ExportFormat[] = ['interview', 'txt', 'markdown', 'html', 'json', 'csv'];

  // 获取所有唯一的发送者
  const uniqueSenders = useMemo(() => {
    return Array.from(
      new Set(
        messages.map(msg => msg.isSender ? '我' : (msg.sender || msg.talker))
      )
    ).sort();
  }, [messages]);

  // 判断是否为系统消息
  const isSystemMessage = (msg: Message): boolean => {
    const sender = msg.sender || msg.talker || '';
    const content = msg.displayContent || msg.content || '';

    // 发送者为"系统消息"
    if (sender === '系统消息' || sender.includes('系统')) {
      return true;
    }

    // 内容包含系统提示关键词
    const systemKeywords = [
      '撤回了一条消息',
      '拍了拍',
      '加入了群聊',
      '退出了群聊',
      '修改群名为',
      '邀请',
      '加入群聊',
      '开启了群聊邀请确认',
      '关闭了群聊邀请确认',
    ];

    if (systemKeywords.some(keyword => content.includes(keyword))) {
      return true;
    }

    // type > 10000 的系统消息
    if (msg.type > 10000) {
      return true;
    }

    return false;
  };

  // 应用过滤和重命名
  const processedMessages = useMemo(() => {
    return messages
      .filter(msg => {
        // 过滤系统消息
        if (filterSystemMessages && isSystemMessage(msg)) {
          return false;
        }
        return true;
      })
      .map(msg => {
        // 应用发送者重命名
        const originalSender = msg.isSender ? '我' : (msg.sender || msg.talker);
        const newSender = senderRenames[originalSender] || originalSender;

        if (newSender !== originalSender) {
          return {
            ...msg,
            sender: msg.isSender ? newSender : msg.sender,
            senderName: newSender,
          };
        }
        return msg;
      });
  }, [messages, filterSystemMessages, senderRenames]);

  useEffect(() => {
    try {
      const content = generateExportContent(selectedFormat, processedMessages);
      setPreviewContent(content);
    }
    catch (error) {
      console.error('Preview error:', error);
      setPreviewContent('预览失败');
    }
  }, [selectedFormat, processedMessages]);

  const handleExport = () => {
    try {
      downloadExport({
        format: selectedFormat,
        messages: processedMessages,
      });
      onOpenChange(false);
    }
    catch (error) {
      console.error('Export error:', error);
    }
  };

  const handleSenderRename = (original: string, newName: string) => {
    setSenderRenames(prev => ({
      ...prev,
      [original]: newName,
    }));
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(previewContent);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
    catch (error) {
      console.error('Copy error:', error);
    }
  };

  const getPreviewLanguage = () => {
    switch (selectedFormat) {
      case 'json':
        return 'json';
      case 'html':
        return 'html';
      case 'markdown':
      case 'interview':
        return 'markdown';
      default:
        return 'text';
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[90vw] max-h-[90vh] flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileText className="h-5 w-5" />
            导出聊天记录
          </DialogTitle>
          <DialogDescription>
            选择导出格式并预览内容，共 {messages.length} 条消息
            {filterSystemMessages && ` (过滤后: ${processedMessages.length} 条)`}
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-[240px,280px,1fr] gap-4 py-4 overflow-hidden flex-1 min-h-0">
          {/* 格式选择 */}
          <div className="flex flex-col min-h-0">
            <Label className="text-base font-semibold mb-4">选择格式</Label>
            <ScrollArea className="flex-1">
              <RadioGroup value={selectedFormat} onValueChange={(v) => setSelectedFormat(v as ExportFormat)}>
                <div className="space-y-2 pr-3">
                  {formats.map(format => (
                    <div key={format} className="flex items-start space-x-2">
                      <RadioGroupItem value={format} id={format} className="mt-1" />
                      <Label
                        htmlFor={format}
                        className="flex-1 cursor-pointer hover:bg-accent rounded p-2 -m-2"
                      >
                        <div className="font-medium text-sm">{getFormatLabel(format)}</div>
                        <div className="text-xs text-muted-foreground">
                          {getFormatDescription(format)}
                        </div>
                      </Label>
                    </div>
                  ))}
                </div>
              </RadioGroup>
            </ScrollArea>
          </div>

          {/* 导出选项 */}
          <div className="flex flex-col min-h-0 border-x px-4">
            <Label className="text-base font-semibold mb-4">导出选项</Label>

            <div className="space-y-3 flex-1 min-h-0 flex flex-col">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="filter-system"
                  checked={filterSystemMessages}
                  onCheckedChange={(checked) => setFilterSystemMessages(checked as boolean)}
                />
                <Label
                  htmlFor="filter-system"
                  className="text-sm font-normal cursor-pointer"
                >
                  过滤系统消息
                </Label>
              </div>

              <Separator />

              <div className="flex-1 min-h-0 flex flex-col">
                <Label className="text-sm font-medium mb-2">发送者重命名</Label>
                <ScrollArea className="flex-1">
                  <div className="space-y-2 pr-3">
                    {uniqueSenders.map(sender => (
                      <div key={sender} className="space-y-1">
                        <Label htmlFor={`rename-${sender}`} className="text-xs text-muted-foreground">
                          {sender}
                        </Label>
                        <Input
                          id={`rename-${sender}`}
                          placeholder={sender}
                          value={senderRenames[sender] || ''}
                          onChange={(e) => handleSenderRename(sender, e.target.value)}
                          className="h-8 text-sm"
                        />
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </div>
            </div>
          </div>

          {/* 预览区域 */}
          <div className="flex flex-col min-h-0">
            <Label className="text-base font-semibold mb-4">
              预览 - {getFormatLabel(selectedFormat)}
            </Label>
            <div className="flex-1 min-h-0 w-full rounded-md border overflow-auto">
              <div className="p-4">
                {previewContent ? (
                  selectedFormat === 'html' ? (
                    <iframe
                      srcDoc={previewContent}
                      className="w-full min-h-[500px] border-0"
                      title="HTML Preview"
                    />
                  ) : (
                    <pre className="text-xs whitespace-pre-wrap break-words font-mono">
                      <code className={`language-${getPreviewLanguage()}`}>
                        {previewContent.length > 50000
                          ? `${previewContent.slice(0, 50000)}\n\n... (内容过长，已截断，实际导出文件将包含完整内容)`
                          : previewContent}
                      </code>
                    </pre>
                  )
                ) : (
                  <div className="text-center text-muted-foreground py-8">
                    加载预览中...
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>

        <DialogFooter className="flex gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button variant="secondary" onClick={handleCopy} disabled={!previewContent}>
            {copied ? (
              <>
                <Check className="mr-2 h-4 w-4" />
                已复制
              </>
            ) : (
              <>
                <Copy className="mr-2 h-4 w-4" />
                复制到剪切板
              </>
            )}
          </Button>
          <Button onClick={handleExport}>
            <Download className="mr-2 h-4 w-4" />
            导出 ({processedMessages.length} 条消息)
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
