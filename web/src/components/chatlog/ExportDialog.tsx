'use client';

import { useState, useEffect } from 'react';
import { Download, FileText } from 'lucide-react';
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

export function ExportDialog({ open, onOpenChange, messages }: ExportDialogProps) {
  const [selectedFormat, setSelectedFormat] = useState<ExportFormat>('txt');
  const [previewContent, setPreviewContent] = useState<string>('');

  const formats: ExportFormat[] = ['txt', 'markdown', 'interview', 'html', 'json', 'csv'];

  useEffect(() => {
    try {
      const content = generateExportContent(selectedFormat, messages);
      setPreviewContent(content);
    }
    catch (error) {
      console.error('Preview error:', error);
      setPreviewContent('预览失败');
    }
  }, [selectedFormat, messages]);

  const handleExport = () => {
    try {
      downloadExport({
        format: selectedFormat,
        messages,
      });
      onOpenChange(false);
    }
    catch (error) {
      console.error('Export error:', error);
    }
  };

  const getPreviewLanguage = () => {
    switch (selectedFormat) {
      case 'json':
        return 'json';
      case 'html':
        return 'html';
      case 'markdown':
        return 'markdown';
      default:
        return 'text';
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-6xl max-h-[85vh]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileText className="h-5 w-5" />
            导出聊天记录
          </DialogTitle>
          <DialogDescription>
            选择导出格式并预览内容，共 {messages.length} 条消息
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-[300px,1fr] gap-6 py-4">
          <div className="space-y-4">
            <Label className="text-base font-semibold">选择格式</Label>
            <RadioGroup value={selectedFormat} onValueChange={(v) => setSelectedFormat(v as ExportFormat)}>
              <div className="space-y-3">
                {formats.map(format => (
                  <div key={format} className="flex items-start space-x-3">
                    <RadioGroupItem value={format} id={format} className="mt-1" />
                    <Label
                      htmlFor={format}
                      className="flex-1 cursor-pointer hover:bg-accent rounded p-2 -m-2"
                    >
                      <div className="font-medium">{getFormatLabel(format)}</div>
                      <div className="text-sm text-muted-foreground">
                        {getFormatDescription(format)}
                      </div>
                    </Label>
                  </div>
                ))}
              </div>
            </RadioGroup>
          </div>

          <div className="space-y-4">
            <Label className="text-base font-semibold">
              预览 - {getFormatLabel(selectedFormat)}
            </Label>
            <ScrollArea className="h-[calc(85vh-240px)] w-full rounded-md border">
              <div className="p-4">
                {previewContent ? (
                  selectedFormat === 'html' ? (
                    <iframe
                      srcDoc={previewContent}
                      className="w-full h-[calc(85vh-280px)] border-0"
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
            </ScrollArea>
          </div>
        </div>

        <DialogFooter className="flex gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleExport}>
            <Download className="mr-2 h-4 w-4" />
            导出
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
