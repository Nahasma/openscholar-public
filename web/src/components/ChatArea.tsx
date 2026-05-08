import { useEffect, useRef } from 'react'
import type { Message } from '../types'

interface ChatAreaProps {
  messages: Message[]
  streamingContent?: string
}

export function ChatArea({ messages, streamingContent }: ChatAreaProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamingContent])

  return (
    <div className="flex-1 overflow-y-auto px-4 py-4">
      {messages.map((msg) => (
        <div
          key={msg.id}
          className="mb-4"
          style={{
            maxWidth: 800,
            marginLeft: msg.role === 'user' ? 'auto' : undefined,
            marginRight: msg.role === 'user' ? 0 : undefined,
          }}
        >
          <div className="text-xs mb-1" style={{ color: 'var(--color-text-muted)' }}>
            {msg.role === 'user' ? 'You' : 'Assistant'}
          </div>
          <div
            className="px-4 py-3 rounded-lg text-sm whitespace-pre-wrap"
            style={{
              backgroundColor: msg.role === 'user' ? 'var(--color-primary)' : 'var(--color-bg-secondary)',
              color: msg.role === 'user' ? '#ffffff' : 'var(--color-text)',
            }}
          >
            {msg.content}
          </div>
        </div>
      ))}

      {streamingContent && (
        <div className="mb-4" style={{ maxWidth: 800 }}>
          <div className="text-xs mb-1" style={{ color: 'var(--color-text-muted)' }}>
            Assistant
          </div>
          <div
            className="px-4 py-3 rounded-lg text-sm whitespace-pre-wrap"
            style={{ backgroundColor: 'var(--color-bg-secondary)', color: 'var(--color-text)' }}
          >
            {streamingContent}
            <span className="animate-pulse">|</span>
          </div>
        </div>
      )}

      <div ref={bottomRef} />
    </div>
  )
}
