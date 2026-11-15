import { useCallback, useEffect, useState } from 'react'
import type { Accent } from '../../../Shared/Components/CardArt/accents'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { Modal } from '../../../Shared/Components/Modal/Modal'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { ApiError } from '../../../api/client'
import {
  approveToolBackground,
  fetchToolBackground,
  revokeToolBackground,
  type BackgroundItem,
  type BackgroundRoute,
} from '../../../api/tools'
import { AppScopes } from '../../../config/security/scopes'
import {
  approvableRoutes,
  approvedBySuffix,
  dialogIntro,
  dialogTitle,
  groupByTarget,
  itemState,
  routeKey,
  routeOf,
  STATE_WORDS,
  summaryText,
  summaryTone,
  type ItemState,
} from './toolBackgroundDisplay'

const stateChip: Record<ItemState, string> = {
  approved: 'bg-green-100 text-green-700',
  pending: 'bg-amber-100 text-amber-700',
  blocked: 'bg-red-100 text-red-700',
}

// The "Unattended access" block of a tool card: only for a tool that asks to use routes of other tools without a signed-in person. The whole block is one
// button that opens the dialog where an administrator approves or takes back each route. Nothing is approved by installing or updating.
export function BackgroundBlock({
  slug,
  name,
  summary,
  accent,
  onChanged,
}: {
  slug: string
  name: string
  summary: { requested: number; approved: number } | null
  accent: Accent
  onChanged: () => void
}) {
  const [open, setOpen] = useState(false)
  const close = useCallback(() => {
    setOpen(false)
  }, [])
  if (summary === null || summary.requested === 0) return null
  const tone = summaryTone(summary)
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        onClick={() => {
          setOpen(true)
        }}
        className="flex w-full items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition hover:brightness-95 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        style={{ background: accent.soft, color: accent.text, borderColor: `${accent.from}66`, outlineColor: accent.to }}
      >
        <span className="shrink-0 text-[11px] font-semibold uppercase tracking-wide">Unattended</span>
        <span className="min-w-0 flex-1 truncate text-xs font-medium text-gray-800">{summaryText(summary)}</span>
        <span
          className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold ${tone === 'green' ? 'bg-white/70' : 'bg-amber-200 text-amber-900'}`}
          aria-hidden="true"
        >
          {tone === 'green' ? 'i' : '!'}
        </span>
      </button>
      <BackgroundDialog open={open} slug={slug} name={name} onClose={close} onChanged={onChanged} />
    </>
  )
}

function BackgroundDialog({
  open,
  slug,
  name,
  onClose,
  onChanged,
}: {
  open: boolean
  slug: string
  name: string
  onClose: () => void
  onChanged: () => void
}) {
  const [items, setItems] = useState<BackgroundItem[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    let current = true
    setItems(null)
    setError('')
    fetchToolBackground(slug)
      .then((list) => {
        if (current) setItems(list)
      })
      .catch((err: unknown) => {
        if (current) setError(err instanceof ApiError ? err.message : 'Failed to load.')
      })
    return () => {
      current = false
    }
  }, [open, slug])

  const change = async (job: Promise<BackgroundItem[]>) => {
    setBusy(true)
    setError('')
    try {
      setItems(await job)
      onChanged()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'That did not work.')
    } finally {
      setBusy(false)
    }
  }

  const waiting = items ? approvableRoutes(items) : []
  return (
    <Modal open={open} title={dialogTitle(name)} onClose={onClose}>
      <p className="mb-3 text-xs leading-relaxed text-gray-600">{dialogIntro(name)}</p>
      {error !== '' && (
        <p role="alert" className="mb-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
          {error}
        </p>
      )}
      {items === null && error === '' && <LoadingSpinner label="Loading…" />}
      {items !== null &&
        groupByTarget(items).map((g) => (
          <section key={g.slug} aria-label={g.name} className="mb-3">
            <h3 className="text-sm font-semibold text-gray-900">
              {g.name} <span className="font-mono text-xs font-normal text-gray-500">{g.slug}</span>
            </h3>
            <ul className="mt-1.5 space-y-1.5">
              {g.items.map((i) => (
                <li key={routeKey(i)} className="flex flex-wrap items-center gap-2 rounded-lg border border-gray-200 px-3 py-2">
                  <span className="font-mono text-xs text-gray-800">
                    {i.method} {i.pathSuffix}
                  </span>
                  <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${stateChip[itemState(i)]}`}>
                    {STATE_WORDS[itemState(i)]}
                  </span>
                  {i.approved && <span className="text-xs text-gray-500">{`Approved${approvedBySuffix(i)}`}</span>}
                  <ScopeGate scopes={[AppScopes.ToolManage]}>
                    <span className="ml-auto">
                      {i.approved ? (
                        <button
                          type="button"
                          disabled={busy}
                          className="text-xs font-medium text-red-600 hover:text-red-800 disabled:opacity-50"
                          onClick={() => {
                            void change(revokeToolBackground(slug, [routeOf(i)]))
                          }}
                        >
                          Take back
                        </button>
                      ) : (
                        <button
                          type="button"
                          disabled={busy || !i.allowedNow}
                          title={i.allowedNow ? undefined : `${i.targetName} does not allow this route to be used in the background.`}
                          className="text-xs font-medium text-gray-900 hover:underline disabled:opacity-40"
                          onClick={() => {
                            void change(approveToolBackground(slug, [routeOf(i)] as BackgroundRoute[]))
                          }}
                        >
                          Approve
                        </button>
                      )}
                    </span>
                  </ScopeGate>
                </li>
              ))}
            </ul>
          </section>
        ))}
      {items !== null && waiting.length > 1 && (
        <ScopeGate scopes={[AppScopes.ToolManage]}>
          <button
            type="button"
            disabled={busy}
            className="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
            onClick={() => {
              void change(approveToolBackground(slug, waiting))
            }}
          >
            {`Approve all ${String(waiting.length)} that can be approved`}
          </button>
        </ScopeGate>
      )}
      {items !== null && items.length === 0 && <p className="text-sm text-gray-500">This tool no longer asks for anything.</p>}
    </Modal>
  )
}
