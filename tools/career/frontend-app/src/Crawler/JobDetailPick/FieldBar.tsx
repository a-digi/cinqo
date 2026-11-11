import { useState, type CSSProperties } from 'react'

import type { PickField, ViewElement } from './jobDetailPick'
import { FIELD_COLOR, FIELD_ORDER, defaultBoxIndex } from './fieldMarks'
import { FIELD_NAMES, shortValue } from './jobDetailPickRules'

type Mode = { kind: 'ask' } | { kind: 'box'; chain: number[]; index: number } | { kind: 'date'; n: number; machine: boolean }

const small = 'rounded-md border border-gray-200 px-2 py-1 text-gray-800 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40'

// The bar a click or a selection in the page of the Job details picker opens (plan 249): what is this text? A field button answers at once. The description then
// shows the box it was read from, with its size and two buttons that climb to the box around it or go back down (the page is the person's to look at; the number is
// a help); a posted date with a machine date asks whether to read the machine date (recommended) or the text. Done closes the bar.
export function FieldBar({
  n,
  byNumber,
  now,
  rule,
  chainOf,
  onAnswer,
  close,
  style,
}: {
  n: number
  byNumber: Map<number, ViewElement>
  // What this element is now.
  now: PickField[]
  // The description is read with a rule: the page cannot set it then.
  rule: boolean
  // The numbered elements from this one up to the outermost the analysis lists.
  chainOf: (n: number) => number[]
  onAnswer: (n: number, field: PickField | null, now: PickField[], attribute?: string) => void
  close: () => void
  style: CSSProperties
}) {
  const [mode, setMode] = useState<Mode>({ kind: 'ask' })
  const el = byNumber.get(n)

  function answer(f: PickField) {
    if (f === 'description') {
      const chain = chainOf(n)
      const index = defaultBoxIndex(chain, byNumber)
      onAnswer(chain[index] ?? n, 'description', now)
      setMode({ kind: 'box', chain, index })
      return
    }
    if (f === 'postedAt' && el?.attribute) {
      onAnswer(n, 'postedAt', now, el.attribute)
      setMode({ kind: 'date', n, machine: true })
      return
    }
    onAnswer(n, f, now)
    close()
  }
  function size(index: number) {
    if (mode.kind !== 'box') return
    onAnswer(mode.chain[index] ?? n, 'description', now)
    setMode({ ...mode, index })
  }
  function dateFrom(machine: boolean) {
    if (mode.kind !== 'date' || !el?.attribute) return
    if (machine) onAnswer(n, 'postedAt', now, el.attribute)
    else onAnswer(n, 'postedAt', now)
    setMode({ ...mode, machine })
  }

  const box = mode.kind === 'box' ? byNumber.get(mode.chain[mode.index] ?? n) : undefined
  return (
    <div
      role="toolbar"
      aria-label="What this part of the page is"
      data-testid="field-bar"
      style={style}
      className="absolute z-10 w-[25rem] max-w-[calc(100%-1rem)] rounded-xl border border-gray-200 bg-white px-2 py-1.5 text-xs shadow-lg"
    >
      {mode.kind === 'ask' && (
        <>
          <p className="truncate px-1 pb-1 text-gray-700" data-testid="field-bar-text">
            “{shortValue(el?.text ?? '', 60)}” is:
          </p>
          <div className="flex flex-wrap items-center gap-1">
            {FIELD_ORDER.map((f) => {
              const locked = f === 'description' && rule
              return (
                <button
                  key={f}
                  type="button"
                  disabled={locked}
                  title={locked ? 'The description is read with a rule: change it under “Show the fields”.' : undefined}
                  aria-pressed={now.includes(f)}
                  className={`flex items-center gap-1 ${small} aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold`}
                  onClick={() => {
                    answer(f)
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
        </>
      )}
      {mode.kind === 'box' && (
        <>
          <p className="px-1 pb-1 text-gray-700" data-testid="field-bar-box">
            The description is this box: <strong>{String(box?.chars ?? 0)} characters</strong>
            <span className="block truncate text-gray-500">“{shortValue(box?.text ?? '', 60)}”</span>
          </p>
          <div className="flex flex-wrap items-center gap-1">
            <button
              type="button"
              className={small}
              disabled={mode.index === 0}
              onClick={() => {
                size(mode.index - 1)
              }}
            >
              Smaller box
            </button>
            <button
              type="button"
              className={small}
              disabled={mode.index >= mode.chain.length - 1}
              onClick={() => {
                size(mode.index + 1)
              }}
            >
              Bigger box
            </button>
            <button type="button" className="rounded-md bg-gray-900 px-2 py-1 text-white hover:bg-gray-800" onClick={close}>
              Done
            </button>
          </div>
        </>
      )}
      {mode.kind === 'date' && (
        <>
          <p className="px-1 pb-1 text-gray-700" data-testid="field-bar-date">
            Read the posted date from:
          </p>
          <div className="flex flex-wrap items-center gap-1">
            <button
              type="button"
              aria-pressed={mode.machine}
              className={`${small} aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold`}
              onClick={() => {
                dateFrom(true)
              }}
            >
              Its machine date (recommended)
            </button>
            <button
              type="button"
              aria-pressed={!mode.machine}
              className={`${small} aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold`}
              onClick={() => {
                dateFrom(false)
              }}
            >
              Its text
            </button>
            <button type="button" className="rounded-md bg-gray-900 px-2 py-1 text-white hover:bg-gray-800" onClick={close}>
              Done
            </button>
          </div>
        </>
      )}
    </div>
  )
}
