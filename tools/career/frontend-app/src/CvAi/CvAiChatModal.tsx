import { useEffect, useRef, useState } from 'react'
import type { CvAiRun } from '../api'
import { fetchConversationMessages, type ConversationMessage } from '../Cinqo/Conversation/conversation'
import { Modal } from '../Shared/Modal/Modal'
import { formatRunTime } from '../PersonaAi/runDisplay'
import { runBadge } from './cvAiRules'

const POLL_MS = 3000

// The chat of one AI CV run, shown in a modal (step 115): the request sent to the AI (folded away,
// it is long) and its answers. While the run is working the chat is read again every few seconds,
// so it follows the run; it is read one last time when the run ends. Plain text only.
export function CvAiChatModal({ run, onClose }: { run: CvAiRun | null; onClose: () => void }) {
  const [messages, setMessages] = useState<ConversationMessage[] | null>(null)
  const [error, setError] = useState('')
  const end = useRef<HTMLDivElement>(null)
  const conversationId = run?.conversationId
  const working = run?.status === 'generating'

  useEffect(() => {
    if (conversationId === undefined) return
    let cancelled = false
    setMessages(null)
    setError('')
    const load = () =>
      fetchConversationMessages(conversationId)
        .then((list) => {
          if (!cancelled) {
            setMessages(list)
            setError('')
          }
        })
        .catch((err: unknown) => {
          if (!cancelled) setError(err instanceof Error ? err.message : String(err))
        })
    void load()
    if (!working) {
      return () => {
        cancelled = true
      }
    }
    const timer = setInterval(() => {
      void load()
    }, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [conversationId, working])

  useEffect(() => {
    end.current?.scrollIntoView({ block: 'end' })
  }, [messages])

  const badge = run ? runBadge(run) : null
  return (
    <Modal open={run !== null} title="Chat with the AI" onClose={onClose} size="wide">
      {run && badge && (
        <>
          <p className="mb-3 flex flex-wrap items-center gap-2 text-xs text-gray-500">
            <span className="rounded-full bg-purple-50 px-2 py-0.5 font-semibold text-purple-800">{badge.label}</span>
            <span>{formatRunTime(run.createdAt)}</span>
            {run.focus !== '' && <span>For: {run.focus}</span>}
          </p>
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {messages === null && error === '' && <p className="text-sm text-gray-500">Loading…</p>}
          {messages !== null && messages.length === 0 && (
            <p className="text-sm text-gray-500">{working ? 'The AI has not answered yet…' : 'This chat has no messages.'}</p>
          )}
          <ul className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
            {messages?.map((m, i) =>
              m.role === 'user' ? (
                <li key={i}>
                  <details className="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600">
                    <summary className="cursor-pointer text-xs font-medium text-gray-500">The request sent to the AI</summary>
                    <p className="mt-2 whitespace-pre-wrap break-words">{m.content}</p>
                  </details>
                </li>
              ) : (
                <li
                  key={i}
                  className={`rounded-xl px-3 py-2 text-sm ${m.failed ? 'bg-red-50 text-red-800' : 'bg-purple-50 text-purple-950'}`}
                >
                  <p className="whitespace-pre-wrap break-words">{m.failed ? (m.error ?? 'The AI turn failed.') : m.content}</p>
                </li>
              ),
            )}
          </ul>
          {working && (
            <p className="mt-3 flex items-center gap-2 text-xs text-purple-800">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-purple-600" aria-hidden="true" />
              The AI is working. This chat updates by itself.
            </p>
          )}
          <div ref={end} />
        </>
      )}
    </Modal>
  )
}
