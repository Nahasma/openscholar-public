import { useState, useCallback } from 'react'
import { Sidebar } from './components/Sidebar'
import { TopBar } from './components/TopBar'
import { ChatArea } from './components/ChatArea'
import { InputBox } from './components/InputBox'
import { WelcomeScreen } from './components/WelcomeScreen'
import { useWebSocket } from './hooks/useWebSocket'
import type { Message, Session, ServerMessage } from './types'

function App() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [currentSessionId, setCurrentSessionId] = useState<string>()
  const [messages, setMessages] = useState<Message[]>([])
  const [streamingContent, setStreamingContent] = useState('')
  const [isProcessing, setIsProcessing] = useState(false)

  const handleServerMessage = useCallback((msg: ServerMessage) => {
    switch (msg.type) {
      case 'content_delta':
        setStreamingContent((prev) => prev + (msg.text || ''))
        break
      case 'complete':
        setStreamingContent((prev) => {
          if (prev) {
            setMessages((msgs) => [
              ...msgs,
              {
                id: Date.now().toString(),
                role: 'assistant',
                sessionId: currentSessionId || '',
                content: prev,
                createdAt: Date.now(),
              },
            ])
          }
          return ''
        })
        setIsProcessing(false)
        break
      case 'error':
        setStreamingContent('')
        setIsProcessing(false)
        break
    }
  }, [currentSessionId])

  const wsUrl = `ws://${window.location.host}/api/chat/ws`
  const { send, isConnected } = useWebSocket({ url: wsUrl, onMessage: handleServerMessage })

  const handleSend = useCallback((text: string) => {
    if (!currentSessionId) return

    setMessages((prev) => [
      ...prev,
      {
        id: Date.now().toString(),
        role: 'user',
        sessionId: currentSessionId,
        content: text,
        createdAt: Date.now(),
      },
    ])

    setIsProcessing(true)
    send({ type: 'message', sessionId: currentSessionId, text })
  }, [currentSessionId, send])

  const handleNewSession = useCallback(() => {
    const id = crypto.randomUUID()
    const session: Session = {
      id,
      title: '新会话',
      messageCount: 0,
      createdAt: Date.now(),
      updatedAt: Date.now(),
    }
    setSessions((prev) => [session, ...prev])
    setCurrentSessionId(id)
    setMessages([])
    setStreamingContent('')
  }, [])

  const handleSelectSession = useCallback((id: string) => {
    setCurrentSessionId(id)
    setMessages([])
    setStreamingContent('')
  }, [])

  return (
    <div className="flex h-screen" style={{ backgroundColor: 'var(--color-bg-main)' }}>
      <Sidebar
        sessions={sessions}
        currentSessionId={currentSessionId}
        onNewSession={handleNewSession}
        onSelectSession={handleSelectSession}
      />
      <div className="flex flex-col flex-1 min-w-0">
        <TopBar
          title={currentSessionId ? '对话' : 'OpenScholar'}
          model={isConnected ? undefined : '未连接'}
        />
        <main className="flex flex-col flex-1 min-h-0">
          {!currentSessionId || (messages.length === 0 && !streamingContent) ? (
            <WelcomeScreen />
          ) : (
            <ChatArea messages={messages} streamingContent={streamingContent} />
          )}
          {currentSessionId && (
            <InputBox onSend={handleSend} disabled={isProcessing} />
          )}
        </main>
      </div>
    </div>
  )
}

export default App
