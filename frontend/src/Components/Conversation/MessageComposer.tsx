import { useState } from 'react'
import type { Platform } from '../../api/platforms'

// Platform <select> + a plain text input — no model picker by default
// (step 1/2 of this plan default `model` to the platform's own
// registered default when omitted). Platform selection is per-message,
// not persisted on the conversation itself, but the caller (
// ConversationPage) remembers the last-picked platform across sends
// for convenience. See plan/ai/conversation/step-05-frontend-chat-ui.md.
export function MessageComposer({
  platforms,
  platform,
  onPlatformChange,
  onSend,
  disabled,
}: {
  platforms: Platform[]
  platform: string
  onPlatformChange: (id: string) => void
  onSend: (content: string) => void
  disabled: boolean
}) {
  const [content, setContent] = useState('')

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
    <form onSubmit={handleSubmit} className="flex items-end gap-2 border-t border-gray-200 p-3">
      <select
        value={platform}
        onChange={(e) => onPlatformChange(e.target.value)}
        disabled={disabled}
        className="rounded border border-gray-300 px-2 py-2 text-sm disabled:opacity-50"
      >
        {platforms.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <textarea
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
        className="min-w-0 flex-1 resize-none rounded border border-gray-300 px-3 py-2 text-sm disabled:opacity-50"
      />
      <button
        type="submit"
        disabled={disabled || content.trim() === ''}
        className="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
      >
        Send
      </button>
    </form>
  )
}
