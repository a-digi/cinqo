import type { CrawlAiAdvice } from './crawlAiAdvice'

// The recommendation pill of plan 213: loud on purpose (a filled pill with a button inside) when it suggests something, quiet green when the
// recommended kind of AI is already in use. Lives inside clickable accordion headers, so every click stops propagation.
export function CrawlAiAdvicePill({ advice, onAction }: { advice: CrawlAiAdvice; onAction: () => void }) {
  if (advice.kind === 'ok') {
    return (
      <span
        data-testid="crawl-ai-advice"
        data-kind={advice.kind}
        className="inline-flex max-w-full items-center gap-1 rounded-full border border-green-200 bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-800"
      >
        <span aria-hidden="true">✓</span>
        <span className="truncate">{advice.text}</span>
      </span>
    )
  }
  return (
    <span
      data-testid="crawl-ai-advice"
      data-kind={advice.kind}
      className="inline-flex max-w-full items-center gap-2 rounded-full bg-violet-600 py-1 pl-3 pr-1 text-xs font-semibold text-white shadow-sm ring-2 ring-violet-300"
    >
      <span aria-hidden="true">✦</span>
      <span className="min-w-0 break-words">{advice.text}</span>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          onAction()
        }}
        className="shrink-0 rounded-full bg-white px-2.5 py-0.5 text-xs font-semibold text-violet-700 hover:bg-violet-50"
      >
        {advice.action === 'get-model' ? 'Get model' : 'Choose'}
      </button>
    </span>
  )
}
