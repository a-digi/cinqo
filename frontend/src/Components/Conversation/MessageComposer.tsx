import { useEffect, useRef, useState } from 'react'

// A plain text input — no platform/model picker here anymore. A
// conversation's platform+model are fixed at creation and can never
// change (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md);
// platformLabel is a read-only display of that fixed choice.
//
// While disabled (a turn is running), the Send button becomes a Stop
// button instead of just going inert — see
// plan/ai/conversation/step-25-cancel-in-progress-turn.md. onStop is
// optional so this component still works anywhere a caller has no
// cancel affordance of its own to offer.
export function MessageComposer({
  platformLabel,
  onSend,
  onStop,
  disabled,
  unavailableReason = null,
}: {
  platformLabel: string
  onSend: (content: string) => void
  onStop?: () => void
  disabled: boolean
  // Set when the key or the model this conversation was created with can not answer any more (plan 243): a notice shows and sending is off until it can
  // again. A turn that is already running is never touched: while `disabled` (a turn running) the Stop button works as before.
  unavailableReason?: string | null
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

  const blocked = unavailableReason !== null && !disabled
  const submit = () => {
    const trimmed = content.trim()
    if (trimmed === '' || disabled || blocked) return
    onSend(trimmed)
    setContent('')
  }

  const handleSubmit = (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault()
    submit()
  }

  return (
    <form onSubmit={handleSubmit} className="border-t border-gray-200 p-3">
      {blocked && (
        <p role="status" className="mb-2 rounded border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs text-amber-800">
          {unavailableReason} You can read this conversation, but not write in it until it can answer again.
        </p>
      )}
      <div className="mb-1.5 truncate text-xs text-gray-500" title={platformLabel}>
        {platformLabel}
      </div>
      <div className="flex items-end gap-2">
        <textarea
          ref={textareaRef}
          value={content}
          onChange={(e) => {
            setContent(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              submit()
            }
          }}
          placeholder="Type a message…"
          rows={1}
          disabled={disabled || blocked}
          className="min-w-0 flex-1 resize-none overflow-y-auto rounded border border-gray-300 px-3 py-2 text-sm disabled:opacity-50"
          style={{ maxHeight: '12rem' }}
        />
        {disabled && onStop ? (
          <button
            type="button"
            onClick={onStop}
            className="rounded border border-red-300 bg-white px-4 py-2 text-sm font-medium text-red-700 hover:bg-red-50"
          >
            Stop
          </button>
        ) : (
          <button
            type="submit"
            disabled={disabled || blocked || content.trim() === ''}
            className="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          >
            Send
          </button>
        )}
      </div>
    </form>
  )
}
