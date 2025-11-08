import type { Message } from '@/libs/ChatlogAPI';
import { format as formatDate } from 'date-fns';

export type ExportFormat = 'json' | 'csv' | 'txt' | 'html' | 'markdown' | 'interview';

export interface ExportOptions {
  format: ExportFormat;
  messages: Message[];
  filename?: string;
}

function formatTime(timestamp: number): string {
  try {
    return formatDate(new Date(timestamp * 1000), 'yyyy-MM-dd HH:mm:ss');
  }
  catch {
    return String(timestamp);
  }
}

export function exportToJSON(messages: Message[]): string {
  return JSON.stringify(messages, null, 2);
}

export function exportToCSV(messages: Message[]): string {
  const headers = ['时间', '发送者', '聊天对象', '消息类型', '内容'];
  const rows = messages.map((msg) => {
    const time = formatTime(msg.createTime);
    const sender = msg.isSender ? '我' : (msg.sender || msg.talker);
    const talker = msg.talker;
    const type = msg.type;
    const content = (msg.displayContent || msg.content || '').replace(/"/g, '""');
    return `"${time}","${sender}","${talker}","${type}","${content}"`;
  });

  return [headers.join(','), ...rows].join('\n');
}

export function exportToText(messages: Message[]): string {
  return messages
    .map((msg) => {
      const time = formatTime(msg.createTime);
      const sender = msg.isSender ? '我' : (msg.sender || msg.talker);
      const content = msg.displayContent || msg.content || '[无内容]';
      return `[${time}] ${sender}: ${content}`;
    })
    .join('\n\n');
}

export function exportToHTML(messages: Message[]): string {
  const messageRows = messages
    .map((msg) => {
      const time = formatTime(msg.createTime);
      const sender = msg.isSender ? '我' : (msg.sender || msg.talker);
      const content = (msg.displayContent || msg.content || '[无内容]')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/\n/g, '<br>');
      const alignment = msg.isSender ? 'right' : 'left';
      const bgColor = msg.isSender ? '#dcf8c6' : '#ffffff';

      return `
    <div style="margin: 10px 0; text-align: ${alignment};">
      <div style="display: inline-block; max-width: 70%; padding: 10px; background: ${bgColor}; border-radius: 8px; text-align: left;">
        <div style="font-weight: bold; font-size: 12px; color: #666; margin-bottom: 5px;">
          ${sender}
        </div>
        <div style="font-size: 14px; word-wrap: break-word;">
          ${content}
        </div>
        <div style="font-size: 11px; color: #999; margin-top: 5px; text-align: right;">
          ${time}
        </div>
      </div>
    </div>`;
    })
    .join('');

  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>聊天记录导出</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      max-width: 800px;
      margin: 0 auto;
      padding: 20px;
      background: #e5ddd5;
    }
    h1 {
      text-align: center;
      color: #075e54;
      border-bottom: 2px solid #075e54;
      padding-bottom: 10px;
    }
    .stats {
      background: #fff;
      padding: 10px;
      border-radius: 8px;
      margin: 20px 0;
      text-align: center;
      font-size: 14px;
      color: #666;
    }
  </style>
</head>
<body>
  <h1>聊天记录导出</h1>
  <div class="stats">
    导出时间: ${formatDate(new Date(), 'yyyy-MM-dd HH:mm:ss')} | 消息数量: ${messages.length}
  </div>
  <div class="messages">
    ${messageRows}
  </div>
</body>
</html>`;
}

function getImageUrl(message: Message): string | null {
  if (message.type !== 3 || !message.contents) {
    return null;
  }

  const baseURL = typeof window !== 'undefined'
    ? (process.env.NEXT_PUBLIC_CHATLOG_API_URL || window.location.origin)
    : (process.env.NEXT_PUBLIC_CHATLOG_API_URL || 'http://localhost:5030');

  if (message.contents.md5) {
    return `${baseURL}/image/${message.contents.md5}`;
  }

  if (message.contents.imgfile) {
    return `${baseURL}/data/${message.contents.imgfile}`;
  }

  return null;
}

export function exportToMarkdown(messages: Message[]): string {
  const header = `# 聊天记录导出

**导出时间**: ${formatDate(new Date(), 'yyyy-MM-dd HH:mm:ss')}
**消息数量**: ${messages.length}

---

`;

  const messageList = messages
    .map((msg) => {
      const time = formatTime(msg.createTime);
      const sender = msg.isSender ? '我' : (msg.sender || msg.talker);
      const isImageMsg = msg.type === 3;
      const typeInfo = msg.type !== 1 && msg.type !== 3 ? ` *(类型: ${msg.type})*` : '';

      let content = msg.displayContent || msg.content || '[无内容]';

      if (isImageMsg) {
        const imageUrl = getImageUrl(msg);
        if (imageUrl) {
          content = `![图片](${imageUrl})`;
        }
        else {
          content = '[图片消息]';
        }
      }

      return `### ${sender}
*${time}*${typeInfo}

${content}

`;
    })
    .join('---\n\n');

  return header + messageList;
}

export function exportToInterview(messages: Message[]): string {
  const parts: string[] = [];
  let lastSender = '';

  for (const msg of messages) {
    const sender = msg.isSender ? '我' : (msg.senderName || msg.sender || msg.talker);
    const isImageMsg = msg.type === 3;

    let content = msg.displayContent || msg.content || '[无内容]';

    if (isImageMsg) {
      const imageUrl = getImageUrl(msg);
      if (imageUrl) {
        content = `![图片](${imageUrl})`;
      }
      else {
        content = '[图片消息]';
      }
    }

    if (sender !== lastSender) {
      parts.push(`**${sender}**: ${content}`);
      lastSender = sender;
    }
    else {
      parts.push(content);
    }
  }

  return parts.join('\n\n');
}

export function generateExportContent(format: ExportFormat, messages: Message[]): string {
  switch (format) {
    case 'json':
      return exportToJSON(messages);
    case 'csv':
      return exportToCSV(messages);
    case 'txt':
      return exportToText(messages);
    case 'html':
      return exportToHTML(messages);
    case 'markdown':
      return exportToMarkdown(messages);
    case 'interview':
      return exportToInterview(messages);
    default:
      throw new Error(`Unsupported format: ${format}`);
  }
}

export function downloadExport({ format, messages, filename }: ExportOptions): void {
  const content = generateExportContent(format, messages);
  const timestamp = formatDate(new Date(), 'yyyyMMdd_HHmmss');
  const extension = (format === 'markdown' || format === 'interview') ? 'md' : format;
  const defaultFilename = `chatlog_${timestamp}.${extension}`;
  const finalFilename = filename || defaultFilename;

  const blob = new Blob([content], { type: getContentType(format) });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = finalFilename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

function getContentType(format: ExportFormat): string {
  switch (format) {
    case 'json':
      return 'application/json';
    case 'csv':
      return 'text/csv';
    case 'txt':
      return 'text/plain';
    case 'html':
      return 'text/html';
    case 'markdown':
    case 'interview':
      return 'text/markdown';
    default:
      return 'text/plain';
  }
}

export function getFormatLabel(format: ExportFormat): string {
  const labels: Record<ExportFormat, string> = {
    json: 'JSON',
    csv: 'CSV (表格)',
    txt: '纯文本',
    html: 'HTML (网页)',
    markdown: 'Markdown',
    interview: '访谈风',
  };
  return labels[format];
}

export function getFormatDescription(format: ExportFormat): string {
  const descriptions: Record<ExportFormat, string> = {
    json: '结构化数据格式，适合程序处理',
    csv: '表格格式，可用 Excel 打开',
    txt: '简单文本格式，易于阅读',
    html: '网页格式，可在浏览器中查看',
    markdown: 'Markdown 格式，适合文档编辑',
    interview: '访谈对话风格，连续发言自动合并',
  };
  return descriptions[format];
}
