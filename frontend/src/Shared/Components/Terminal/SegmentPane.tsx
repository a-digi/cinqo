import { useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import type { LineSegmentState } from './useLineSegment'
import {
  keyToMove,
  lastSegmentOffset,
  parsePositionInput,
  prevSegmentOffset,
  segmentOffsetFromFraction,
  splitMatches,
  thumb,
} from './terminalRules'

const buttonClass =
  'rounded px-2 py-1 text-xs text-gray-300 transition hover:bg-white/10 hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400 disabled:opacity-40 disabled:hover:bg-transparent'

// One long line shown a segment at a time (the file reader's segment view): for a line the window can only show
// the start of, such as a file that is one line. The text wraps (it is one long line), a bar along the side
// stands for the whole line in bytes, and a search match inside the line is marked.
export function SegmentPane({
  state,
  name,
  paneHeight,
  query,
  caseSensitive,
  onSearch,
  onMatch,
  onBack,
}: {
  state: LineSegmentState
  name: string
  paneHeight: string
  query: string
  caseSensitive: boolean
  onSearch: () => void
  onMatch: (direction: 1 | -1) => void
  onBack: () => void
}) {
  const { segment, loading, error } = state
  const [positionInput, setPositionInput] = useState('')
  const [drag, setDrag] = useState<number | null>(null)
  const paneRef = useRef<HTMLDivElement>(null)
  const trackRef = useRef<HTMLDivElement>(null)

  const lineBytes = segment?.lineBytes ?? 0
  const offset = segment?.offset ?? 0
  const size = segment ? Math.max(1, segment.nextOffset - segment.offset) : 1
  // how far a "next segment" moves: what the segment on screen holds (a full one is 16 KiB)
  const step = Math.max(size, 1)

  // A new segment starts at the top of the pane, or at the first match in it.
  useLayoutEffect(() => {
    const pane = paneRef.current
    if (!pane) return
    const mark = pane.querySelector('mark')
    if (mark) mark.scrollIntoView({ block: 'center' })
    else pane.scrollTop = 0
  }, [segment, query])

  const next = () => {
    if (segment?.hasMore) state.goTo(segment.nextOffset)
  }
  const prev = () => {
    if (offset > 0) state.goTo(prevSegmentOffset(offset, step))
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const move = keyToMove(e.key, e.shiftKey)
    if (!move) return
    if (move.kind === 'scroll') {
      e.preventDefault()
      paneRef.current?.scrollBy({ top: move.lines * 20 })
      return
    }
    e.preventDefault()
    if (move.kind === 'page') {
      if (move.direction === 1) next()
      else prev()
    } else if (move.kind === 'edge') state.goTo(move.edge === 'top' ? 0 : lastSegmentOffset(lineBytes, step))
    else if (move.kind === 'search') onSearch()
    else onMatch(move.direction)
  }

  const fractionAt = (e: PointerEvent) => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (!rect || rect.height === 0) return 0
    return (e.clientY - rect.top) / rect.height
  }
  const t = thumb((drag ?? offset) + 1, size, lineBytes)
  const known = segment !== null && lineBytes > 0

  return (
    <>
      <div className="flex flex-wrap items-center gap-2 border-b border-white/10 bg-[#0e131a] px-3 py-1.5">
        <button type="button" className={`${buttonClass} border border-white/20`} onClick={onBack} title="Back to the lines">
          ← Back to lines
        </button>
        <span className="mx-1 h-4 w-px bg-white/10" aria-hidden="true" />
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            state.goTo(0)
          }}
          disabled={offset === 0}
          aria-label="First segment"
          title="Home or g"
        >
          ⏮ Start
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={prev}
          disabled={offset === 0}
          aria-label="Previous segment"
          title="Page Up or b"
        >
          ◀ Prev
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={next}
          disabled={!segment?.hasMore}
          aria-label="Next segment"
          title="Page Down, space or f"
        >
          Next ▶
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            state.goTo(lastSegmentOffset(lineBytes, step))
          }}
          disabled={!known || !segment.hasMore}
          aria-label="Last segment"
          title="End or G"
        >
          End ⏭
        </button>
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            const to = parsePositionInput(positionInput, lineBytes, step)
            if (to !== null) state.goTo(to)
          }}
        >
          <label htmlFor={`pos-${name}`} className="sr-only">
            Go to a position in the line, in bytes or as a percentage
          </label>
          <input
            id={`pos-${name}`}
            value={positionInput}
            onChange={(e) => {
              setPositionInput(e.target.value)
            }}
            placeholder="byte or 50%"
            className="w-28 rounded border border-white/15 bg-black/30 px-2 py-1 text-xs text-gray-100 placeholder:text-gray-400 focus:border-emerald-400 focus:outline-none"
          />
          <button type="submit" className={buttonClass} disabled={!known || parsePositionInput(positionInput, lineBytes, step) === null}>
            Go
          </button>
        </form>
      </div>

      {error ? (
        <div role="alert" className={`flex ${paneHeight} flex-col items-center justify-center gap-3 px-6 text-center text-sm text-red-300`}>
          <p>
            <span aria-hidden="true">✗ </span>
            {error}
          </p>
          <button type="button" className={`${buttonClass} border border-white/20`} onClick={state.retry}>
            Try again
          </button>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
          {/* A scrollable text region must be reachable by keyboard (so it can be scrolled and paged), which these two rules cannot express. */}
          {/* eslint-disable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/no-noninteractive-tabindex */}
          <div
            ref={paneRef}
            role="region"
            aria-label={`${name}, line ${state.line ?? ''}, contents`}
            aria-busy={loading}
            tabIndex={0}
            onKeyDown={onKey}
            className={`${paneHeight} min-w-0 flex-1 overflow-auto px-4 py-2 text-[13px] leading-5 outline-none selection:bg-emerald-400/30 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-emerald-400 ${
              loading && segment ? 'opacity-70' : ''
            }`}
          >
            {!segment && <p className="py-8 text-center text-[#8b95a5]">reading…</p>}
            {segment?.text === '' && <p className="py-8 text-center text-[#8b95a5]">Nothing at this position.</p>}
            {segment && segment.text !== '' && (
              <pre className="whitespace-pre-wrap break-all font-mono">
                {query === ''
                  ? segment.text
                  : splitMatches(segment.text, query, caseSensitive).map((part, i) =>
                      part.hit ? (
                        <mark key={i} className="rounded-sm bg-amber-400/40 text-amber-50">
                          {part.text}
                        </mark>
                      ) : (
                        part.text
                      ),
                    )}
              </pre>
            )}
          </div>
          {/* eslint-enable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/no-noninteractive-tabindex */}

          <div
            ref={trackRef}
            role="slider"
            tabIndex={0}
            aria-label="Position in the line"
            aria-orientation="vertical"
            aria-valuemin={0}
            aria-valuemax={Math.max(0, lineBytes)}
            aria-valuenow={offset}
            aria-disabled={!known}
            title={known ? 'Drag or click to move through the line' : 'Available when the line has loaded'}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next()
              else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') prev()
              else if (e.key === 'Home') state.goTo(0)
              else if (e.key === 'End') state.goTo(lastSegmentOffset(lineBytes, step))
              else return
              e.preventDefault()
            }}
            onPointerDown={(e) => {
              if (!known) return
              e.currentTarget.setPointerCapture(e.pointerId)
              setDrag(segmentOffsetFromFraction(fractionAt(e), lineBytes, step))
            }}
            onPointerMove={(e) => {
              if (drag !== null) setDrag(segmentOffsetFromFraction(fractionAt(e), lineBytes, step))
            }}
            onPointerUp={(e) => {
              if (drag === null) return
              state.goTo(segmentOffsetFromFraction(fractionAt(e), lineBytes, step))
              setDrag(null)
            }}
            onPointerCancel={() => {
              setDrag(null)
            }}
            className={`relative w-3 shrink-0 touch-none border-l border-white/10 bg-black/30 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400 ${
              known ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'
            }`}
          >
            <div
              className="absolute inset-x-0.5 rounded-sm bg-emerald-400/60"
              style={{ top: `${t.top * 100}%`, height: `${t.height * 100}%` }}
              aria-hidden="true"
            />
          </div>
        </div>
      )}
    </>
  )
}
