import { useCallback, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Tool } from '../../../api/tools'
import type { Accent } from '../../../Shared/Components/CardArt/accents'
import { Modal } from '../../../Shared/Components/Modal/Modal'
import {
  blockTitle,
  describeDependency,
  stateDot,
  stateLabel,
  worstState,
  type DependencyState,
  type DependencyView,
} from './toolDependencyDisplay'

const REPOSITORIES_PATH = '/admin/tool-repositories'

const stateChip: Record<DependencyState, string> = {
  ready: 'bg-green-100 text-green-700',
  disabled: 'bg-amber-100 text-amber-700',
  too_old: 'bg-amber-100 text-amber-700',
  missing: 'bg-red-100 text-red-700',
}

// The "Needs" block of a tool card (plan 217): one pill per tool this one cannot work without, each with a status dot, tinted with the card's accent.
// The whole block is one button that opens the dialog with the details. Nothing is drawn for a tool that needs nothing.
export function DependenciesBlock({ tool, tools, accent }: { tool: Tool; tools: readonly Tool[]; accent: Accent }) {
  const [open, setOpen] = useState(false)
  const close = useCallback(() => {
    setOpen(false)
  }, [])
  if (tool.requiredTools.length === 0) return null
  const views = tool.requiredTools.map((r) => describeDependency(r, tools))
  const worst = worstState(views.map((v) => v.state))
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        title={blockTitle(tool.name, views)}
        onClick={() => {
          setOpen(true)
        }}
        className="flex w-full items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition hover:brightness-95 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        style={{ background: accent.soft, color: accent.text, borderColor: `${accent.from}66`, outlineColor: accent.to }}
      >
        <span className="shrink-0 text-[11px] font-semibold uppercase tracking-wide">Needs</span>
        <span className="flex min-w-0 flex-1 flex-wrap gap-1.5">
          {views.map((v) => (
            <span
              key={v.slug}
              className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-white/80 px-2 py-0.5 text-xs font-medium text-gray-800 ring-1 ring-black/5"
            >
              <span className={`h-2 w-2 shrink-0 rounded-full ${stateDot[v.state]}`} aria-hidden="true" />
              <span className="truncate">{v.name}</span>
              <span className="sr-only">: {stateLabel[v.state]}</span>
            </span>
          ))}
        </span>
        <span
          className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold ${worst === 'ready' ? 'bg-white/70' : 'bg-amber-200 text-amber-900'}`}
          aria-hidden="true"
        >
          {worst === 'ready' ? 'i' : '!'}
        </span>
      </button>
      <Modal open={open} title={`What ${tool.name} needs`} onClose={close}>
        <ul className="space-y-3">
          {views.map((v) => (
            <DependencyItem key={v.slug} view={v} />
          ))}
        </ul>
      </Modal>
    </>
  )
}

function DependencyItem({ view }: { view: DependencyView }) {
  return (
    <li className="rounded-xl border border-gray-200 p-3.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-gray-900">{view.name}</p>
          <p className="truncate font-mono text-xs text-gray-500">{view.slug}</p>
        </div>
        <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${stateChip[view.state]}`}>{stateLabel[view.state]}</span>
      </div>
      <dl className="mt-2.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-gray-500">Needs</dt>
        <dd className="text-gray-800">{view.needs}</dd>
        <dt className="text-gray-500">Installed</dt>
        <dd className="text-gray-800">{view.installed}</dd>
        {view.byline !== null && (
          <>
            <dt className="text-gray-500">Made by</dt>
            <dd className="truncate text-gray-800">{view.byline.replace(/^by /, '')}</dd>
          </>
        )}
      </dl>
      {view.description !== '' && (
        <p className="mt-2.5 rounded-lg bg-gray-50 px-3 py-2 text-xs leading-relaxed text-gray-700">{view.description}</p>
      )}
      <p
        className={`mt-2.5 text-xs font-medium ${view.state === 'ready' ? 'text-green-700' : view.state === 'missing' ? 'text-red-700' : 'text-amber-700'}`}
      >
        {view.verdict}
        {view.state === 'missing' && (
          <>
            {' '}
            <Link to={REPOSITORIES_PATH} className="underline">
              Open the repositories
            </Link>
          </>
        )}
      </p>
    </li>
  )
}
