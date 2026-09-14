import { useEffect, useRef, useState } from 'react'

// A plain text input — no platform/model picker here anymore. A
// conversation's platform+model are fixed at creation and can never
// change (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md);
// platformLabel is a read-only display of that fixed choice.
export function MessageComposer({
  platformLabel,
  onSend,
  disabled,
}: {
  platformLabel: string
  onSend: (content: string) => void
  disabled: boolean
}) {
  const [content, setContent] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Auto-grow: reset to 'auto' first so scrollHeight reflects the new
  // content only, not the box's own prior (possibly taller) height —
  // otherwise it could never shrink back down when text is deleted.
  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [content])

  const submit = () => {
    const trimmed = content.trim()
    if (trimmed === '' || disabled) return
    onSend(trimmed)
    setContent('')
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    submit()
  }

  return (
    <form onSubmit={handleSubmit} className="border-t border-gray-200 p-3">
      <div className="mb-1.5 truncate text-xs text-gray-500" title={platformLabel}>
        {platformLabel}
      </div>
      <div className="flex items-end gap-2">
        <textarea
          ref={textareaRef}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              submit()
            }
          }}
          placeholder="Type a message…"
          rows={1}
          disabled={disabled}
          className="min-w-0 flex-1 resize-none overflow-y-auto rounded border border-gray-300 px-3 py-2 text-sm disabled:opacity-50"
          style={{ maxHeight: '12rem' }}
        />
        <button
          type="submit"
          disabled={disabled || content.trim() === ''}
          className="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          Send
        </button>
      </div>
    </form>
  )
}
