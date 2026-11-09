import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { pillText, pillTitle, resourceLevel, type ResourceLevel } from './resourceLevel'
import { useResources } from './useResources'

const PILL: Record<ResourceLevel, string> = {
  unknown: 'bg-gray-100 text-gray-600',
  ok: 'bg-green-50 text-green-800',
  orange: 'bg-orange-100 text-orange-800',
  red: 'bg-red-100 text-red-800',
}

// The slim bar under the top bar (plan 222): what the computer is doing, always visible. The pill at the right turns orange or
// red when the memory is tight; the colour is never the only sign (the words change and a "!" appears).
export function ResourceBar() {
  const state = useResources()
  if (state.kind === 'loading' || state.kind === 'hidden') return null
  if (state.kind === 'unknown') {
    return <Bar left={null} pill="Memory: unknown" level="unknown" title="The computer's memory could not be read." />
  }
  const r = state.resources
  const level = resourceLevel(r)
  const left = [`Cinqo ${formatBytes(r.cinqo.appBytes)}`]
  if (r.memory.swapUsedBytes > 0) left.push(`Swap ${formatBytes(r.memory.swapUsedBytes)}`)
  return <Bar left={left.join(' · ')} pill={pillText(r, level)} level={level} title={pillTitle(r)} />
}

function Bar({ left, pill, level, title }: { left: string | null; pill: string; level: ResourceLevel; title: string }) {
  return (
    <div
      role="status"
      aria-label="Computer resources"
      className="flex h-8 shrink-0 items-center justify-between gap-3 border-b border-gray-200 bg-white px-4 text-xs text-gray-600"
    >
      <span className="hidden truncate sm:inline">{left}</span>
      <span title={title} className={`ml-auto inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 font-medium ${PILL[level]}`}>
        {(level === 'orange' || level === 'red') && <span aria-hidden="true">!</span>}
        {pill}
      </span>
    </div>
  )
}
