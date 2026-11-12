import { ACCENTS, gradient } from '../../Components/JobsPage/jobsTheme'
import type { JourneyStep } from './infographicTypes'
import { Reveal } from './Reveal'

export interface FirstStepSpec {
  id: string
  title: string
  text: string
  // The real button or page to use for the step, as it is named on the page.
  click: string
}

// "Your first ...": the steps as a numbered path with what the page can tell (done, next, or not known), and a count.
export function FirstStepsPath({
  specs,
  steps,
  progress,
  nextId,
}: {
  specs: FirstStepSpec[]
  steps: JourneyStep[]
  progress: { done: number; total: number }
  nextId: string | null
}) {
  const accent = ACCENTS[1]
  return (
    <div className="mx-auto w-full max-w-3xl">
      <p role="status" className="mb-3 text-center text-sm text-gray-600">
        {progress.done === progress.total ? (
          <span className="font-semibold text-green-700">Every step you can see on this page is done.</span>
        ) : (
          <>
            <span className="font-semibold text-gray-900">
              {progress.done} of {progress.total}
            </span>{' '}
            steps you can see on this page are done.
          </>
        )}
      </p>
      <ol className="space-y-2.5">
        {specs.map((s, i) => {
          const state = steps.find((x) => x.id === s.id)
          const done = state?.done === true
          const next = nextId === s.id
          return (
            <Reveal as="li" key={s.id} delay={i}>
              <div
                className="flex items-start gap-3 rounded-2xl border bg-white p-3 shadow-sm"
                style={{ borderColor: done ? accent.from : next ? ACCENTS[2].from : '#e5e7eb', background: done ? accent.soft : undefined }}
                data-state={done ? 'done' : next ? 'next' : 'open'}
              >
                <span
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white"
                  style={{ background: done ? gradient(accent) : next ? gradient(ACCENTS[2]) : '#9ca3af' }}
                  aria-hidden="true"
                >
                  {done ? '✓' : i + 1}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-gray-900">{s.title}</span>
                    {done && <span className="rounded-full bg-green-100 px-2 py-0.5 text-[11px] font-medium text-green-800">Done</span>}
                    {next && <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800">Next</span>}
                  </div>
                  <p className="text-xs text-gray-600">{s.text}</p>
                </div>
                <span className="shrink-0 rounded-full border border-gray-200 bg-white px-2.5 py-1 text-xs font-medium text-gray-700">
                  {s.click}
                </span>
              </div>
            </Reveal>
          )
        })}
      </ol>
    </div>
  )
}
