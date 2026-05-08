interface TopBarProps {
  title?: string
  model?: string
}

export function TopBar({ title, model }: TopBarProps) {
  return (
    <header
      className="flex items-center justify-between px-4 border-b"
      style={{
        height: 48,
        backgroundColor: 'var(--color-bg-main)',
        borderColor: 'var(--color-border)',
      }}
    >
      <span className="text-sm font-medium" style={{ color: 'var(--color-text)' }}>
        {title || 'OpenScholar'}
      </span>
      {model && (
        <span className="text-xs px-2 py-1 rounded" style={{ backgroundColor: 'var(--color-bg-secondary)', color: 'var(--color-text-muted)' }}>
          {model}
        </span>
      )}
    </header>
  )
}
