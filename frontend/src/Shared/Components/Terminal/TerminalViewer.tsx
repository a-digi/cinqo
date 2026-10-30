import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { readWindowSize, readWrap, saveWindowSize, saveWrap } from './terminalPrefs'
import { useFileSearch } from './useFileSearch'
import { useFileWindow } from './useFileWindow'
import {
  WINDOW_SIZES,
  fromFraction,
  hiddenBytes,
  indexProgress,
  isSearchable,
  keyToMove,
  matchStatus,
  parseLineInput,
  promptText,
  splitMatches,
  statusText,
  thumb,
  type Highlighter,
  type HighlightKind,
  type WindowSize,
} from './terminalRules'

export interface TerminalViewerProps {
  // Which file: a source id and a reference the server resolves (never a path).
  source: string
  fileRef: string
  title?: string
  // Where the window size starts. Left out, the size the person last picked is used (remembered per browser).
  initialWindow?: WindowSize
  // Colours the structure of a line (a banner, an error…); none by default.
  highlight?: Highlighter
  // The height of the text area (a Tailwind class); 60% of the screen by default.
  paneHeight?: string
  className?: string
}

const encoder = new TextEncoder()
const encodedLength = (text: string) => encoder.encode(text).length

const HIGHLIGHT_CLASS: Record<HighlightKind, string> = {
  section: 'font-semibold text-sky-300',
  error: 'font-semibold text-red-400',
  role: 'text-emerald-300',
  tool: 'text-amber-200',
  meta: 'text-[#8b95a5]',
}

const buttonClass =
  'rounded px-2 py-1 text-xs text-gray-300 transition hover:bg-white/10 hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400 disabled:opacity-40 disabled:hover:bg-transparent'

const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

// Reads a big text file a window of up to 500 lines at a time (the file reader, plan/ai/file-reader), in a
// terminal: a title bar, the lines with a line-number gutter, and a prompt line at the foot that says where
// you are. The person picks the window size and where it sits: Home / End, PgUp / PgDn (and the `less`
// keys), a line number, or the bar along the side that stands for the whole file.
export function TerminalViewer({
  source,
  fileRef,
  title,
  initialWindow,
  highlight,
  paneHeight = 'h-[60vh]',
  className = '',
}: TerminalViewerProps) {
  const [size, setSize] = useState<WindowSize>(() => initialWindow ?? readWindowSize())
  const [wrap, setWrap] = useState<boolean>(readWrap)
  const [lineInput, setLineInput] = useState('')
  // The window start shown on the minimap while its thumb is being dragged, before it is committed.
  const [drag, setDrag] = useState<number | null>(null)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchInput, setSearchInput] = useState('')
  const [sensitive, setSensitive] = useState(false)
  // A short note in the prompt line ("Copied 200 lines"). Local on purpose: the viewer also runs inside an installed
  // tool's page, which has none of the app's providers.
  const [notice, setNotice] = useState<{ text: string; ok: boolean } | null>(null)
  const state = useFileWindow(source, fileRef, size)
  const search = useFileSearch(source, fileRef)
  const { info, lines, shownFrom, target, loading, error } = state
  const paneRef = useRef<HTMLDivElement>(null)
  const trackRef = useRef<HTMLDivElement>(null)
  const targetRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const total = info?.totalLines ?? 0
  const complete = info?.complete ?? false
  const shown = lines.length
  const name = title ?? fileRef

  // A new window starts at the top of the pane, unless the person asked for a line in it: then that line is
  // brought into view.
  useLayoutEffect(() => {
    const pane = paneRef.current
    if (!pane) return
    if (targetRef.current) targetRef.current.scrollIntoView({ block: 'center', behavior: reducedMotion() ? 'auto' : 'smooth' })
    else pane.scrollTop = 0
  }, [shownFrom, shown, target])

  // Stepping to a match brings its line into view (and marks it, like "go to line").
  const matchLine = search.index >= 0 ? (search.matches[search.index]?.n ?? null) : null
  const goTo = state.goTo
  useEffect(() => {
    if (matchLine !== null) goTo(matchLine)
  }, [matchLine, goTo])

  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => {
      setNotice(null)
    }, 3000)
    return () => {
      window.clearTimeout(timer)
    }
  }, [notice])

  useEffect(() => {
    saveWindowSize(size)
  }, [size])
  useEffect(() => {
    saveWrap(wrap)
  }, [wrap])

  const status = useMemo(
    () => statusText({ from: shownFrom, shown, total, complete, sizeBytes: info?.sizeBytes ?? 0 }),
    [shownFrom, shown, total, complete, info?.sizeBytes],
  )
  const t = thumb(drag ?? shownFrom, shown, total)

  const onPaneKey = (e: KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const move = keyToMove(e.key, e.shiftKey)
    if (!move) return
    e.preventDefault()
    if (move.kind === 'page') state.page(move.direction)
    else if (move.kind === 'edge') {
      if (move.edge === 'top') state.top()
      else state.bottom()
    } else if (move.kind === 'scroll') paneRef.current?.scrollBy({ top: move.lines * 20 })
    else if (move.kind === 'search') openSearch()
    else if (move.direction === 1) search.next()
    else search.prev()
  }

  const openSearch = () => {
    setSearchOpen(true)
    window.setTimeout(() => {
      searchRef.current?.focus()
      searchRef.current?.select()
    }, 0)
  }
  const closeSearch = () => {
    setSearchOpen(false)
    search.clear()
    paneRef.current?.focus()
  }
  // Switching "match case" while a search is showing searches again with the new setting.
  const toggleCase = () => {
    const next = !sensitive
    setSensitive(next)
    if (search.query !== '' && isSearchable(searchInput, encodedLength)) search.run(searchInput, next, shownFrom)
  }
  const submitSearch = (backwards: boolean) => {
    if (!isSearchable(searchInput, encodedLength)) return
    // the same text again steps on; a new text (or case) searches afresh from the top
    if (search.query === searchInput && search.caseSensitive === sensitive && !search.error) {
      if (backwards) search.prev()
      else search.next()
    } else search.run(searchInput, sensitive, shownFrom)
  }

  const jump = () => {
    const line = parseLineInput(lineInput, complete ? total : 0)
    if (line === null) return
    state.goTo(line)
  }

  const copy = () => {
    navigator.clipboard.writeText(lines.map((l) => l.text).join('\n')).then(
      () => {
        setNotice({ text: `Copied ${shown} lines`, ok: true })
      },
      () => {
        setNotice({ text: 'Could not copy. Select the text and copy it by hand.', ok: false })
      },
    )
  }

  const fractionAt = (e: PointerEvent) => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (!rect || rect.height === 0) return 0
    return (e.clientY - rect.top) / rect.height
  }

  const percent = info && !info.complete ? indexProgress(info.scannedBytes, info.sizeBytes) : null

  return (
    <div
      className={`flex flex-col overflow-hidden rounded-xl bg-[#0b0f14] font-mono text-gray-200 shadow-xl ring-1 ring-black/50 ${className}`}
    >
      {/* title bar */}
      <div className="flex items-center gap-3 border-b border-white/10 bg-[#11161d] px-4 py-2.5">
        <span className="flex gap-1.5" aria-hidden="true">
          <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
          <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
          <span className="h-3 w-3 rounded-full bg-[#28c840]" />
        </span>
        <span className="min-w-0 flex-1 truncate text-center text-xs text-gray-300" title={name}>
          {name}
        </span>
        {percent !== null ? (
          <span className="text-xs text-amber-300">indexing {percent}%</span>
        ) : (
          <span className="w-12" aria-hidden="true" />
        )}
      </div>

      {/* toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-white/10 px-3 py-1.5">
        <div role="group" aria-label="Window size" className="flex items-center gap-0.5">
          {WINDOW_SIZES.map((n) => (
            <button
              key={n}
              type="button"
              aria-pressed={n === size}
              onClick={() => {
                setSize(n)
              }}
              className={`${buttonClass} ${n === size ? 'bg-emerald-400/20 text-emerald-200' : ''}`}
            >
              {n}
            </button>
          ))}
          <span className="ml-1 text-xs text-[#8b95a5]">lines</span>
        </div>
        <span className="mx-1 h-4 w-px bg-white/10" aria-hidden="true" />
        <button type="button" className={buttonClass} onClick={state.top} aria-label="Go to the start of the file" title="Home or g">
          ⏮ Start
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            state.page(-1)
          }}
          aria-label="Previous window"
          title="Page Up or b"
        >
          ◀ Prev
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            state.page(1)
          }}
          aria-label="Next window"
          title="Page Down, space or f"
        >
          Next ▶
        </button>
        <button type="button" className={buttonClass} onClick={state.bottom} aria-label="Go to the end of the file" title="End or G">
          End ⏭
        </button>
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            jump()
          }}
        >
          <label htmlFor={`goto-${fileRef}`} className="sr-only">
            Go to line
          </label>
          <input
            id={`goto-${fileRef}`}
            value={lineInput}
            onChange={(e) => {
              setLineInput(e.target.value)
            }}
            inputMode="numeric"
            placeholder="line #"
            className="w-24 rounded border border-white/15 bg-black/30 px-2 py-1 text-xs text-gray-100 placeholder:text-gray-400 focus:border-emerald-400 focus:outline-none"
          />
          <button type="submit" className={buttonClass} disabled={parseLineInput(lineInput, 0) === null}>
            Go
          </button>
        </form>
        <div className="ml-auto flex items-center gap-1">
          <button
            type="button"
            aria-pressed={searchOpen}
            aria-expanded={searchOpen}
            onClick={() => {
              if (searchOpen) closeSearch()
              else openSearch()
            }}
            className={`${buttonClass} ${searchOpen ? 'bg-emerald-400/20 text-emerald-200' : ''}`}
            title="Search (/)"
          >
            Search
          </button>
          <button
            type="button"
            aria-pressed={wrap}
            onClick={() => {
              setWrap((w) => !w)
            }}
            className={`${buttonClass} ${wrap ? 'bg-emerald-400/20 text-emerald-200' : ''}`}
            title="Wrap long lines"
          >
            Wrap
          </button>
          <button type="button" className={buttonClass} onClick={copy} disabled={shown === 0}>
            Copy visible
          </button>
        </div>
      </div>

      {searchOpen && (
        <form
          role="search"
          className="flex flex-wrap items-center gap-2 border-b border-white/10 bg-[#0e131a] px-3 py-1.5"
          onSubmit={(e) => {
            e.preventDefault()
            submitSearch(false)
          }}
        >
          <label htmlFor={`search-${fileRef}`} className="sr-only">
            Search the file
          </label>
          <span className="text-emerald-400" aria-hidden="true">
            /
          </span>
          <input
            id={`search-${fileRef}`}
            ref={searchRef}
            value={searchInput}
            onChange={(e) => {
              setSearchInput(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation()
                closeSearch()
              } else if (e.key === 'Enter' && e.shiftKey) {
                e.preventDefault()
                submitSearch(true)
              }
            }}
            maxLength={200}
            placeholder="search this file…"
            className="min-w-48 flex-1 rounded border border-white/15 bg-black/30 px-2 py-1 text-xs text-gray-100 placeholder:text-gray-400 focus:border-emerald-400 focus:outline-none"
          />
          <button
            type="button"
            aria-pressed={sensitive}
            onClick={toggleCase}
            className={`${buttonClass} ${sensitive ? 'bg-emerald-400/20 text-emerald-200' : ''}`}
            title="Match case"
          >
            Aa
          </button>
          <button type="submit" className={buttonClass} disabled={!isSearchable(searchInput, encodedLength)}>
            Find
          </button>
          <button
            type="button"
            className={buttonClass}
            onClick={search.prev}
            disabled={search.index <= 0}
            aria-label="Previous match"
            title="Previous match (N, Shift+Enter)"
          >
            ▲
          </button>
          <button
            type="button"
            className={buttonClass}
            onClick={search.next}
            disabled={search.matches.length === 0 || (search.index >= search.matches.length - 1 && search.done) || search.searching}
            aria-label="Next match"
            title="Next match (n, Enter)"
          >
            ▼
          </button>
          <button type="button" className={buttonClass} onClick={closeSearch} aria-label="Close search">
            ✕
          </button>
          {search.query !== '' && (
            <span className="order-last basis-full truncate text-xs text-[#8b95a5]" aria-live="polite">
              {matchStatus({
                count: search.matches.length,
                index: search.index,
                done: search.done,
                searching: search.searching,
                failed: search.error !== null,
              })}
              {search.index >= 0 ? `: ${search.matches[search.index]?.preview ?? ''}` : ''}
            </span>
          )}
        </form>
      )}

      {percent !== null && (
        <div className="h-0.5 bg-white/5" aria-hidden="true">
          <div className="h-full bg-amber-300/70 transition-all" style={{ width: `${percent}%` }} />
        </div>
      )}

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
            aria-label={`${name} contents`}
            aria-busy={loading}
            tabIndex={0}
            onKeyDown={onPaneKey}
            className={`${paneHeight} min-w-0 flex-1 overflow-auto py-2 text-[13px] leading-5 outline-none selection:bg-emerald-400/30 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-emerald-400 ${
              loading && shown > 0 ? 'opacity-70' : ''
            }`}
          >
            {shown === 0 && !loading && <p className="px-4 py-8 text-center text-[#8b95a5]">This file has no lines.</p>}
            {shown === 0 && loading && <p className="px-4 py-8 text-center text-[#8b95a5]">reading…</p>}
            {lines.map((l) => {
              const hidden = hiddenBytes(l, encodedLength)
              const kind = highlight?.(l.text) ?? null
              const isTarget = l.n === target
              return (
                <div
                  key={l.n}
                  ref={isTarget ? targetRef : undefined}
                  className={`flex ${wrap ? '' : 'min-w-max'} ${isTarget ? 'bg-emerald-400/15' : 'hover:bg-white/[0.03]'}`}
                >
                  <span
                    className={`sticky left-0 w-16 shrink-0 select-none pr-3 text-right ${
                      isTarget ? 'bg-[#10241d] text-emerald-300' : 'bg-[#0b0f14] text-[#8b95a5]'
                    }`}
                  >
                    {l.n}
                  </span>
                  <span
                    className={`pr-4 ${wrap ? 'min-w-0 whitespace-pre-wrap break-words' : 'whitespace-pre'} ${kind ? HIGHLIGHT_CLASS[kind] : ''}`}
                  >
                    {search.query === ''
                      ? l.text
                      : splitMatches(l.text, search.query, search.caseSensitive).map((part, i) =>
                          part.hit ? (
                            <mark key={i} className="rounded-sm bg-amber-400/40 text-amber-50">
                              {part.text}
                            </mark>
                          ) : (
                            part.text
                          ),
                        )}
                    {hidden > 0 && <span className="ml-2 select-none text-amber-300">… +{hidden.toLocaleString('en-US')} bytes</span>}
                  </span>
                </div>
              )
            })}
          </div>
          {/* eslint-enable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/no-noninteractive-tabindex */}

          <div
            ref={trackRef}
            role="slider"
            tabIndex={0}
            aria-label="Position in the file"
            aria-orientation="vertical"
            aria-valuemin={1}
            aria-valuemax={Math.max(1, total)}
            aria-valuenow={shownFrom}
            aria-disabled={!complete}
            title={complete ? 'Drag or click to move through the file' : 'Available when indexing is done'}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown' || e.key === 'ArrowRight') state.page(1)
              else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') state.page(-1)
              else if (e.key === 'Home') state.top()
              else if (e.key === 'End') state.bottom()
              else return
              e.preventDefault()
            }}
            onPointerDown={(e) => {
              if (!complete) return
              e.currentTarget.setPointerCapture(e.pointerId)
              setDrag(fromFraction(fractionAt(e), total, size))
            }}
            onPointerMove={(e) => {
              if (drag !== null) setDrag(fromFraction(fractionAt(e), total, size))
            }}
            onPointerUp={(e) => {
              if (drag === null) return
              state.moveToFraction(fractionAt(e))
              setDrag(null)
            }}
            onPointerCancel={() => {
              setDrag(null)
            }}
            className={`relative w-3 shrink-0 touch-none border-l border-white/10 bg-black/30 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400 ${
              complete ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'
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

      {/* prompt line: where you are, in terminal words */}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-white/10 bg-[#11161d] px-4 py-2 text-xs">
        <span className="min-w-0 truncate text-emerald-400">
          {promptText(name, shownFrom, shown)}
          <span className="cinqo-caret ml-1 inline-block h-3.5 w-1.5 translate-y-0.5 bg-emerald-400" aria-hidden="true" />
        </span>
        <span className={notice ? (notice.ok ? 'text-emerald-300' : 'text-red-300') : 'text-[#8b95a5]'} aria-live="polite">
          {notice ? notice.text : info ? status : 'opening…'}
        </span>
      </div>
    </div>
  )
}
