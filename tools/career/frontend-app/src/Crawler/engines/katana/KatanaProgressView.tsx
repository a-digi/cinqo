import { useEffect } from 'react'
import type { ProgressProps } from '../types'
import { katanaProgress, type StepState } from './progress'

const DOT: Record<StepState, string> = {
  done: 'bg-green-500',
  working: 'bg-amber-500 animate-pulse',
  waiting: 'bg-blue-300',
  todo: 'bg-gray-300',
}
const TEXT: Record<StepState, string> = {
  done: 'text-green-800',
  working: 'text-amber-800',
  waiting: 'text-blue-800',
  todo: 'text-gray-500',
}

// The Katana engine's progress under a link's pipeline line: three steps (job list rule, jobs crawled, job details), each marked done, working,
// waiting or not started, with a headline that names what the AI is on right now. While the AI works the page is asked to reload every three
// seconds, so a step turns to done as soon as the AI has saved it (the backend raises nothing a page could listen to).
export function KatanaProgressView({ link, onReload }: ProgressProps) {
  const progress = katanaProgress(link)
  useEffect(() => {
    if (!progress.aiRunning) return
    const timer = setInterval(onReload, 3000)
    return () => {
      clearInterval(timer)
    }
  }, [progress.aiRunning, onReload])

  return (
    <div data-testid="katana-progress" className="mb-2 rounded-xl bg-gray-50 px-3 py-2 text-xs">
      <p role="status" className={progress.aiRunning ? 'font-medium text-amber-800' : 'text-gray-600'}>
        {progress.headline}
      </p>
      <ol className="mt-1.5 flex flex-wrap gap-x-5 gap-y-1">
        {progress.steps.map((step, i) => (
          <li key={step.id} className="flex items-center gap-1.5" data-step={step.id} data-state={step.state}>
            <span aria-hidden className={`inline-block h-2.5 w-2.5 rounded-full ${DOT[step.state]}`} />
            <span className={`font-medium ${TEXT[step.state]}`}>
              {i + 1}. {step.label}
            </span>
            <span className="text-gray-500">{step.note}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}
