import { useState } from 'react'
import type { CrawlAiAdvice } from './crawlAiAdvice'

function InfoIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
      <path
        fillRule="evenodd"
        d="M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Zm-7-4a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM9 9a1 1 0 0 0 0 2v3a1 1 0 0 0 1 1h1a1 1 0 1 0 0-2v-3a1 1 0 0 0-1-1H9Z"
        clipRule="evenodd"
      />
    </svg>
  )
}

// The recommendation pill of plan 213 (links only): a short pill with an info icon, loud when it suggests something and quiet green when the
// recommended kind of AI is in use. Clicking it shows the explanation and, when there is something to do, the button for it. Lives inside
// clickable accordion headers, so every click stops propagation.
export function CrawlAiAdvicePill({ advice, onAction }: { advice: CrawlAiAdvice; onAction: () => void }) {
  const [open, setOpen] = useState(false)
  const quiet = advice.kind === 'ok'
  return (
    <div
      role="presentation"
      data-testid="crawl-ai-advice"
      data-kind={advice.kind}
      // Inside a clickable accordion header: a click anywhere in the pill or its text must not toggle the accordion.
      onClick={(e) => {
        e.stopPropagation()
      }}
      className="max-w-full"
    >
      <button
        type="button"
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation()
          setOpen((v) => !v)
        }}
        className={`inline-flex max-w-full items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${
          quiet
            ? 'border border-green-200 bg-green-50 text-green-800 hover:bg-green-100'
            : 'bg-violet-600 text-white shadow-sm ring-2 ring-violet-300 hover:bg-violet-700'
        }`}
      >
        <span className="truncate">{advice.text}</span>
        <InfoIcon />
      </button>
      {open && (
        <div role="note" className="mt-1.5 max-w-md rounded-lg border border-gray-200 bg-white p-3 text-xs text-gray-700 shadow-sm">
          <p>{advice.info}</p>
          {advice.action !== 'none' && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onAction()
              }}
              className="mt-2 rounded-full bg-violet-600 px-3 py-1 text-xs font-semibold text-white hover:bg-violet-700"
            >
              {advice.action === 'suggest-model' ? 'Ask Cinqo to download it' : 'Choose'}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
