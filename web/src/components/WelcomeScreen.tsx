export function WelcomeScreen() {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4" style={{ color: 'var(--color-text-muted)' }}>
      <h1 className="text-2xl font-bold" style={{ color: 'var(--color-primary)' }}>
        OpenScholar
      </h1>
      <p className="text-sm">AI-Powered Academic Writing Assistant</p>
      <div className="text-sm mt-4 space-y-1 text-center">
        <p>撰写和编辑 LaTeX 论文</p>
        <p>管理 BibTeX 参考文献</p>
        <p>投稿前全面检查</p>
      </div>
      <p className="text-xs mt-8" style={{ color: 'var(--color-text-muted)' }}>
        在下方输入消息开始对话
      </p>
    </div>
  )
}
