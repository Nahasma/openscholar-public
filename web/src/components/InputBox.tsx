import { useState, useRef, useCallback } from 'react'

interface InputBoxProps {
  onSend: (text: string) => void
  disabled?: boolean
}

export function InputBox({ onSend, disabled }: InputBoxProps) {
  const [text, setText] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const handleSubmit = useCallback(() => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setText('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [text, disabled, onSend])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }, [handleSubmit])

  const handleInput = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setText(e.target.value)
    // Auto-resize
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [])

  return (
    <div
      className="px-4 py-3 border-t"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-bg-main)' }}
    >
      <div
        className="flex items-end gap-2 rounded-lg border px-3 py-2"
        style={{
          borderColor: 'var(--color-border)',
          backgroundColor: 'var(--color-bg-secondary)',
          maxWidth: 800,
          margin: '0 auto',
        }}
      >
        <textarea
          ref={textareaRef}
          value={text}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          disabled={disabled}
          rows={1}
          className="flex-1 resize-none outline-none text-sm bg-transparent"
          style={{ color: 'var(--color-text)', minHeight: 24, maxHeight: 200 }}
        />
        <button
          onClick={handleSubmit}
          disabled={disabled || !text.trim()}
          className="px-3 py-1 rounded text-sm text-white cursor-pointer transition-opacity disabled:opacity-50"
          style={{ backgroundColor: 'var(--color-primary)' }}
        >
          发送
        </button>
      </div>
    </div>
  )
}
