import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import type { InstallPlan, InstallPlanAlternative } from '../../../api/toolRepositories'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { alternativeText, canConfirm, markText, needsTick, problemTitle, sizeText, stepReason, stepTitle } from './installPlanDisplay'

export interface InstallPlanDialogProps {
  plan: InstallPlan
  rootToolId: string
  // A note above the list: why it is shown again, or what went wrong last time.
  notice?: string
  working: boolean
  onConfirm: () => void
  onCancel: () => void
  // Chosen when the person prefers to update another tool, which makes room for this one.
  onUseAlternative?: (alternative: InstallPlanAlternative) => void
}

// The confirmation of an install or update together with the tools it needs: the ordered steps, who made them and where they come from, one tick for
// each tool from another repository, and the problems that stop the whole thing. Drawn on document.body like every other modal.
export function InstallPlanDialog({ plan, rootToolId, notice, working, onConfirm, onCancel, onUseAlternative }: InstallPlanDialogProps) {
  const [ticked, setTicked] = useState<Set<string>>(new Set())
  // A different plan (it changed while it was checked) must be ticked again.
  useEffect(() => {
    setTicked(new Set())
  }, [plan])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !working) onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [working, onCancel])

  const ok = canConfirm(plan, ticked)
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="plan-dialog-title"
        className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-2xl bg-white shadow-xl"
      >
        <h2 id="plan-dialog-title" className="border-b border-gray-100 px-5 py-4 text-base font-semibold text-gray-900">
          {plan.steps.length > 1 ? 'Install with the tools it needs' : 'Install'}
        </h2>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-5 py-4">
          {notice && (
            <p role="alert" className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">
              {notice}
            </p>
          )}
          {plan.problems.length > 0 && (
            <ul className="space-y-2">
              {plan.problems.map((p, i) => (
                <li key={i} className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
                  <p className="font-semibold">{problemTitle(p)}</p>
                  <p>{p.message}</p>
                </li>
              ))}
            </ul>
          )}
          {onUseAlternative &&
            plan.alternatives.map((alt) => (
              <div key={alt.toolId} className="rounded-xl bg-blue-50 px-3 py-2.5 text-sm text-blue-900">
                <p>{alternativeText(alt)}</p>
                <button
                  type="button"
                  disabled={working}
                  onClick={() => {
                    onUseAlternative(alt)
                  }}
                  className="mt-2 rounded-md bg-blue-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-800 disabled:opacity-50"
                >
                  Look at updating {alt.name}
                </button>
              </div>
            ))}
          {plan.steps.length > 0 && (
            <>
              <p className="text-sm text-gray-600">
                These are done one after the other, in this order. If one fails, the ones before it stay installed.
              </p>
              <ol className="space-y-2">
                {plan.steps.map((s, i) => (
                  <li key={s.toolId + s.action} className="rounded-xl bg-gray-50 px-3 py-2.5">
                    <div className="flex items-start justify-between gap-2">
                      <p className="text-sm font-medium text-gray-900">
                        {i + 1}. {stepTitle(s)}
                      </p>
                      <span className="shrink-0 text-xs text-gray-500">{s.action === 'enable' ? '' : sizeText(s.sizeBytes)}</span>
                    </div>
                    {stepReason(s, rootToolId) && <p className="text-xs text-gray-500">{stepReason(s, rootToolId)}</p>}
                    {(s.authors.length > 0 || s.license) && (
                      <p className="text-xs text-gray-500">
                        {s.authors.length > 0 && `By ${s.authors.join(', ')}`}
                        {s.authors.length > 0 && s.license && ' · '}
                        {s.license}
                      </p>
                    )}
                    {s.marks.length > 0 && (
                      <ul className="mt-1.5 flex flex-wrap gap-1.5">
                        {s.marks.map((m) => (
                          <li key={m}>
                            <Pill variant={m === 'restarts' || m === 'was_disabled' ? 'gray' : 'amber'}>{markText(m, s)}</Pill>
                          </li>
                        ))}
                      </ul>
                    )}
                    {needsTick(s) && (
                      <label className="mt-2 flex items-center gap-2 text-sm text-gray-800">
                        <input
                          type="checkbox"
                          checked={ticked.has(s.toolId)}
                          onChange={(e) => {
                            setTicked((prev) => {
                              const next = new Set(prev)
                              if (e.target.checked) next.add(s.toolId)
                              else next.delete(s.toolId)
                              return next
                            })
                          }}
                        />
                        Also install {s.name} from {s.repositoryName}
                      </label>
                    )}
                  </li>
                ))}
              </ol>
              <p className="text-xs text-gray-500">Total download: {sizeText(plan.totalBytes)}</p>
            </>
          )}
        </div>
        <div className="flex justify-end gap-2 border-t border-gray-100 px-5 py-3">
          <button
            type="button"
            disabled={working}
            onClick={onCancel}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={working || !ok}
            onClick={onConfirm}
            className="rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {working ? 'Working…' : 'Confirm'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
