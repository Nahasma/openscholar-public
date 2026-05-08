import type { Session } from '../types'

interface SidebarProps {
  sessions: Session[]
  currentSessionId?: string
  onNewSession: () => void
  onSelectSession: (id: string) => void
}

export function Sidebar({ sessions, currentSessionId, onNewSession, onSelectSession }: SidebarProps) {
  return (
    <aside
      className="flex flex-col h-full border-r"
      style={{
        width: 260,
        backgroundColor: 'var(--color-bg-sidebar)',
        borderColor: 'var(--color-border)',
      }}
    >
      {/* Logo */}
      <div className="flex items-center gap-2 px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
        <span className="text-lg font-bold" style={{ color: 'var(--color-primary)' }}>
          OpenScholar
        </span>
      </div>

      {/* New Session Button */}
      <div className="px-3 py-2">
        <button
          onClick={onNewSession}
          className="w-full px-3 py-2 text-sm rounded-md border cursor-pointer transition-colors hover:opacity-80"
          style={{
            color: 'var(--color-primary)',
            borderColor: 'var(--color-primary)',
            backgroundColor: 'transparent',
          }}
        >
          + 新建会话
        </button>
      </div>

      {/* Session List */}
      <div className="flex-1 overflow-y-auto px-2">
        {sessions.map((session) => (
          <div
            key={session.id}
            onClick={() => onSelectSession(session.id)}
            className="px-3 py-2 rounded-md cursor-pointer text-sm truncate mb-1 transition-colors"
            style={{
              backgroundColor: session.id === currentSessionId ? 'var(--color-bg-secondary)' : 'transparent',
              color: 'var(--color-text)',
            }}
          >
            {session.title || '新会话'}
          </div>
        ))}
      </div>

      {/* Settings */}
      <div className="px-3 py-2 border-t text-sm" style={{ borderColor: 'var(--color-border)', color: 'var(--color-text-muted)' }}>
        Settings
      </div>
    </aside>
  )
}
