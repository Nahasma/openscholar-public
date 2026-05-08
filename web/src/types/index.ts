export interface Session {
  id: string
  title: string
  messageCount: number
  createdAt: number
  updatedAt: number
}

export interface Message {
  id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  sessionId: string
  content: string
  toolCalls?: ToolCall[]
  createdAt: number
}

export interface ToolCall {
  id: string
  name: string
  input: string
  finished: boolean
}

export interface ServerMessage {
  type: 'content_delta' | 'tool_use_start' | 'tool_use_delta' | 'tool_result' | 'complete' | 'error'
  text?: string
  toolName?: string
  toolId?: string
  input?: string
  content?: string
  error?: string
  done?: boolean
}
