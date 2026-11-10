import { useEffect, useMemo, useState } from 'react'

import { PageFrame } from '../PageFrame/PageFrame'
import { FRAME_STYLE } from '../pageView'
import type { PickField, ViewElement } from './jobDetailPick'
import { FIELD_NAMES, shortValue, type Picks, type RuleDraft } from './jobDetailPickRules'
import {
  drawnFields,
  FIELD_CLASSES,
  FIELD_COLOR,
  FIELD_MARK_STYLE,
  FIELD_ORDER,
  fieldsByNumber,
  numberedAround,
  unknownSelectors,
  viewByNumber,
} from './fieldMarks'

// The page of a job as the Browser tool read it (plan 249): the same frame as the Job list's (a copy with no script, scrolling, pop-ups hidden), with the picks drawn on it
// and a bar that asks what a clicked or selected element is: the title, the company, the location, the posted date or the description. Nothing is mapped until the person
// answers. The panel's picks stay the one place the choices live; this only shows them and reports what the person did.
export function DetailPageView({
  snapshotId,
  view,
  picks,
  rule,
  large,
  reveal,
  onAnswer,
}: {
  snapshotId: string
  view: ViewElement[]
  picks: Picks
  // The description is read with a rule: the page cannot set the description then (the rule's boxes are drawn).
  rule: RuleDraft | null
  large: boolean
  reveal: { numbers: number[]; seq: number } | null
  // The person answered for the element n: a field, or null to clear what it is now (the fields it is now are given).
  onAnswer: (n: number, field: PickField | null, now: PickField[]) => void
}) {
  const byNumber = useMemo(() => viewByNumber(view), [view])
  // Selectors the analysis does not know (a stored pick) are asked of the copy itself, once it is loaded.
  const [doc, setDoc] = useState<Document | null>(null)
  const [found, setFound] = useState<Map<string, number>>(new Map())
  const unknown = useMemo(() => unknownSelectors(view, picks, rule), [view, picks, rule])
  useEffect(() => {
    if (!doc) return
    const next = new Map<string, number>()
    for (const s of unknown) {
      try {
        const n = numberedAround(doc.querySelector(s))
        if (n !== undefined) next.set(s, n)
      } catch {
        // a selector the page copy cannot use is simply not drawn
      }
    }
    setFound((prev) => (prev.size === next.size && [...next].every(([k, v]) => prev.get(k) === v) ? prev : next))
  }, [doc, unknown])

  const fields = useMemo(() => fieldsByNumber(view, picks, rule, found), [view, picks, rule, found])
  const drawn = useMemo(() => drawnFields(fields), [fields])

  return (
    <PageFrame
      snapshotId={snapshotId}
      large={large}
      testId="job-detail-page-view"
      hint="Scroll the page, then click or select the text of a field and say what it is."
      attribute="data-cinqo-e"
      style={FRAME_STYLE + FIELD_MARK_STYLE}
      marks={drawn}
      markClasses={FIELD_CLASSES}
      reveal={reveal}
      onReady={(_present, d) => {
        setDoc(d)
      }}
      clickTargets={(target) => {
        const n = numberedAround(target)
        return n !== undefined && byNumber.has(n) ? [String(n)] : []
      }}
      onSelection={(range) => {
        const n = numberedAround(range.commonAncestorContainer)
        return n !== undefined && byNumber.has(n) ? [String(n)] : []
      }}
      renderBar={(bar, style, close) => {
        const n = Number(bar.targets[0])
        const el = byNumber.get(n)
        const now = fields.get(n) ?? []
        return (
          <div
            role="toolbar"
            aria-label="What this part of the page is"
            data-testid="field-bar"
            style={style}
            className="absolute z-10 w-[25rem] max-w-[calc(100%-1rem)] rounded-xl border border-gray-200 bg-white px-2 py-1.5 text-xs shadow-lg"
          >
            <p className="truncate px-1 pb-1 text-gray-700" data-testid="field-bar-text">
              “{shortValue(el?.text ?? '', 60)}” is:
            </p>
            <div className="flex flex-wrap items-center gap-1">
              {FIELD_ORDER.map((f) => {
                const locked = f === 'description' && rule !== null
                return (
                  <button
                    key={f}
                    type="button"
                    disabled={locked}
                    title={locked ? 'The description is read with a rule: change it under “Show the fields”.' : undefined}
                    aria-pressed={now.includes(f)}
                    className="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-gray-800 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold"
                    onClick={() => {
                      onAnswer(n, f, now)
                      close()
                    }}
                  >
                    <span aria-hidden="true" className="h-2 w-2 rounded-full" style={{ backgroundColor: FIELD_COLOR[f] }} />
                    {FIELD_NAMES[f]}
                  </button>
                )
              })}
              <button
                type="button"
                className="rounded-md border border-gray-200 px-2 py-1 text-gray-700 hover:bg-gray-100"
                onClick={() => {
                  onAnswer(n, null, now)
                  close()
                }}
              >
                Clear
              </button>
            </div>
          </div>
        )
      }}
    />
  )
}
