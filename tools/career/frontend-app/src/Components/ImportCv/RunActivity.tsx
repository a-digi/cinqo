import { useEffect, useState } from 'react'
import type { CVImportRun } from '../../api'
import { activityItems, elapsedLabel } from './runActivityRules'

// What a generating import shows (plan 209): the AI is working (an animated indicator and the time it has taken) and what has been stored so far,
// ticking off as the import goes. The chat of the run shows nothing while a page drives the import in code, and little while the AI works in
// steps in other conversations, so this is where a person sees that something is happening.
export function RunActivity({ run }: { run: CVImportRun }) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const timer = setInterval(() => {
      setNow(new Date())
    }, 1000)
    return () => {
      clearInterval(timer)
    }
  }, [])
  const items = activityItems(run.progress)
  const elapsed = elapsedLabel(run.createdAt, now)
  return (
    <div role="status" aria-live="polite">
      <p className="flex items-center gap-2 text-sm font-medium text-gray-800">
        <span className="inline-flex gap-1" aria-hidden="true">
          <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.3s]" />
          <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.15s]" />
          <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500" />
        </span>
        The AI is working on this CV{elapsed !== '' ? ` · ${elapsed}` : ''}
      </p>
      <ul className="mt-3 grid grid-cols-1 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        {items.map((item) => (
          <li key={item.key} className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                item.state === 'done' ? 'bg-green-600 text-white' : item.state === 'working' ? 'animate-pulse bg-blue-600 text-white' : 'bg-gray-200 text-gray-500'
              }`}
            >
              {item.state === 'done' ? '✓' : item.state === 'working' ? '…' : ''}
            </span>
            <span className={item.state === 'todo' ? 'text-gray-400' : item.state === 'working' ? 'font-medium text-blue-900' : 'text-gray-800'}>
              {item.label}
              {item.detail !== '' && <span className="ml-1 text-xs text-gray-500">{item.detail}</span>}
              {item.state === 'working' && <span className="sr-only"> (in progress)</span>}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
