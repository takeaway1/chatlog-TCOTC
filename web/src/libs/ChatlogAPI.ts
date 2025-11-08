/**
 * Chatlog API Client
 * Communicates with Go backend HTTP API
 */

// Use environment variable if available, otherwise fall back to current origin in production
// or localhost:5030 in development
const API_BASE_URL = typeof window !== 'undefined'
  ? (process.env.NEXT_PUBLIC_CHATLOG_API_URL || window.location.origin)
  : (process.env.NEXT_PUBLIC_CHATLOG_API_URL || 'http://localhost:5030');

export interface MessageContents {
  imgfile?: string;
  md5?: string;
  thumb?: string;
  title?: string;
  url?: string;
  refer?: any;
}

export interface Message {
  localId: number;
  talkerId: number;
  msgSvrId: string;
  type: number;
  subType: number;
  isSender: boolean;
  createTime: number;
  sequence: number;
  statusEx: number;
  flagEx: number;
  status: number;
  msgSeq: number;
  msgServerSeq: number;
  msgSequence: number;
  createTimestamp: string;
  talker: string;
  content: string;
  compressedContent: string;
  bytesExtra: string;
  displayContent: string;
  sender: string;
  senderName?: string;
  senderAvatar?: string;
  contents?: MessageContents;
}

export interface Contact {
  userName: string;
  alias: string;
  remark: string;
  nickName: string;
  isFriend: boolean;
  bigHeadImgUrl?: string;
  smallHeadImgUrl?: string;
  headImgMd5?: string;
}

export interface ContactList {
  items: Contact[];
  total: number;
}

export interface ChatRoom {
  name: string;
  users: string[];
  remark: string;
  nickName: string;
  owner: string;
}

export interface ChatRoomList {
  items: ChatRoom[];
  total: number;
}

export interface Session {
  userName: string;
  nOrder: number;
  parentRef: string;
  nickName: string;
  content: string;
  nTime: string;
  nUnReadCount: number;
  avatarUrl?: string;
  isTopPinned?: boolean;
  isHidden?: boolean;
  sortOrder?: number;
}

export interface SessionList {
  items: Session[];
  total: number;
}

export interface GetChatlogParams {
  time?: string;
  talker?: string;
  sender?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
  format?: 'json' | 'csv' | 'text';
}

export interface GetContactsParams {
  keyword?: string;
  limit?: number;
  offset?: number;
  format?: 'json' | 'csv';
}

export interface GetChatRoomsParams {
  keyword?: string;
  limit?: number;
  offset?: number;
  format?: 'json' | 'csv';
}

export interface GetSessionsParams {
  keyword?: string;
  limit?: number;
  offset?: number;
  format?: 'json' | 'csv' | 'text';
}

class ChatlogAPIClient {
  private baseURL: string;

  constructor(baseURL: string = API_BASE_URL) {
    this.baseURL = baseURL;
  }

  private buildURL(path: string, params?: Record<string, any>): string {
    const url = new URL(path, this.baseURL);
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
          url.searchParams.append(key, String(value));
        }
      });
    }
    return url.toString();
  }

  private async fetchJSON<T>(url: string): Promise<T> {
    const response = await fetch(url, {
      headers: {
        Accept: 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return response.json();
  }

  /**
   * Get chat messages
   */
  async getChatlog(params: GetChatlogParams): Promise<Message[]> {
    const url = this.buildURL('/api/v1/chatlog', { ...params, format: 'json' });
    return this.fetchJSON<Message[]>(url);
  }

  /**
   * Get contacts list
   */
  async getContacts(params: GetContactsParams = {}): Promise<ContactList> {
    const url = this.buildURL('/api/v1/contact', { ...params, format: 'json' });
    return this.fetchJSON<ContactList>(url);
  }

  /**
   * Get chatrooms list
   */
  async getChatRooms(params: GetChatRoomsParams = {}): Promise<ChatRoomList> {
    const url = this.buildURL('/api/v1/chatroom', { ...params, format: 'json' });
    return this.fetchJSON<ChatRoomList>(url);
  }

  /**
   * Get sessions list
   */
  async getSessions(params: GetSessionsParams = {}): Promise<SessionList> {
    const url = this.buildURL('/api/v1/session', { ...params, format: 'json' });
    return this.fetchJSON<SessionList>(url);
  }

  /**
   * Get image URL
   */
  getImageURL(key: string): string {
    return `${this.baseURL}/image/${key}`;
  }

  /**
   * Get video URL
   */
  getVideoURL(key: string): string {
    return `${this.baseURL}/video/${key}`;
  }

  /**
   * Get voice URL
   */
  getVoiceURL(key: string): string {
    return `${this.baseURL}/voice/${key}`;
  }

  /**
   * Get file URL
   */
  getFileURL(key: string): string {
    return `${this.baseURL}/file/${key}`;
  }

  /**
   * Get media data URL
   */
  getMediaDataURL(path: string): string {
    return `${this.baseURL}/data/${path}`;
  }
}

export const chatlogAPI = new ChatlogAPIClient();
