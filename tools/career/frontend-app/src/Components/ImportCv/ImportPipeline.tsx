import type { Stage, StageStatus } from './pipelineRules'

const STYLE: Record<StageStatus, { box: string; dot: string; text: string; sub: string }> = {
  done: { box: 'border-green-200 bg-green-50', dot: 'bg-green-600 text-white', text: 'text-green-800', sub: 'text-green-700' },
  current: {
    box: 'border-blue-300 bg-blue-50 ring-2 ring-blue-200',
    dot: 'bg-blue-600 text-white',
    text: 'text-blue-900',
    sub: 'text-blue-700',
  },
  failed: { box: 'border-red-300 bg-red-50', dot: 'bg-red-600 text-white', text: 'text-red-800', sub: 'text-red-700' },
  todo: { box: 'border-gray-200 bg-white', dot: 'bg-gray-200 text-gray-500', text: 'text-gray-500', sub: 'text-gray-400' },
}

const WORD: Record<StageStatus, string> = { done: 'Done', current: 'In progress', failed: 'Failed', todo: '' }

// The Import from CV page's overview: four connected cards (Upload, Analyze, Review,
// Save) in a row above the content. Finished stages tick, the current one is
// highlighted (and pulses while the page is working), a failed one turns red. A failed
// Analyze stage offers "Start over".
export function ImportPipeline({ stages, onStartOver }: { stages: Stage[]; onStartOver: () => void }) {
  return (
    <ol className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4" aria-label="Import progress">
      {stages.map((stage, i) => {
        const style = STYLE[stage.status]
        return (
          <li
            key={stage.key}
            aria-current={stage.status === 'current' ? 'step' : undefined}
            className={`relative rounded-2xl border p-3 ${style.box} ${stage.busy ? 'animate-pulse' : ''}`}
          >
            {i < stages.length - 1 && (
              <span
                aria-hidden="true"
                className={`absolute -right-3 top-1/2 hidden h-0.5 w-3 sm:block ${stage.status === 'done' ? 'bg-green-400' : 'bg-gray-200'}`}
              />
            )}
            <div className="flex items-center gap-2">
              <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold ${style.dot}`}>
                {stage.status === 'done' ? '✓' : stage.status === 'failed' ? '!' : i + 1}
              </span>
              <span className={`text-sm font-semibold ${style.text}`}>{stage.label}</span>
            </div>
            <div className={`mt-1 text-xs ${style.sub}`}>{WORD[stage.status] || stage.hint}</div>
            {stage.status === 'failed' && stage.key === 'analyze' && (
              <button
                type="button"
                onClick={onStartOver}
                className="mt-2 rounded-full border border-red-300 bg-white px-2.5 py-0.5 text-xs font-medium text-red-700 hover:bg-red-50"
              >
                Start over
              </button>
            )}
          </li>
        )
      })}
    </ol>
  )
}
