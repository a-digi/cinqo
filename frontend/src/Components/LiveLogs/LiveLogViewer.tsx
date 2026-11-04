import { useCallback, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { HIGHLIGHT_CLASS, type Highlighter } from './highlight'
import { announce, isNearBottom, jumpLabel, keyToAction, loopNote, unseenLines } from './scrollRules'
import { lineNumber, statusText } from './tailRules'
import type { LiveFeed } from './types'
import { useLiveFeed } from './useLiveFeed'

export interface LiveLogViewerProps {
  // The log to follow (liveFeedFor(provider, ref)); a different feed starts afresh.
  feed: LiveFeed
  // What the title bar and the prompt call it.
  name: string
  // Colours the structure of a line (a banner, an error…); none by default.
  highlight?: Highlighter
  // The height of the text area (a Tailwind class); 60% of the screen by default.
  paneHeight?: string
  className?: string
}

const buttonClass =
  'rounded px-2 py-1 text-xs text-gray-300 transition hover:bg-white/10 hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400 disabled:opacity-40 disabled:hover:bg-transparent'

// Follows a log while it is written (tail -f), in a terminal: a title bar with a LIVE mark, the newest lines at the bottom with a gutter of
// numbers that count from when the following began (a live log has no line numbers), a pause, and a prompt line that says what is held.
// The pane stays on the newest line; scrolling up lets go of it and says how many lines arrived, until "jump to latest" or the bottom.
// It shares no code with the file viewer next to it (api/src/livelog, plan/ai/file-reader/live-follow.md).
export function LiveLogViewer({ feed, name, highlight, paneHeight = 'h-[60vh]', className = '' }: LiveLogViewerProps) {
  const { tail, loop, paused, pause, resume, restart } = useLiveFeed(feed)
  const [wrap, setWrap] = useState(false)
  const [pinned, setPinned] = useState(true)
  const [seenSeq, setSeenSeq] = useState(0)
  const [notice, setNotice] = useState<string | null>(null)
  const paneRef = useRef<HTMLDivElement>(null)

  const lines = tail.lines
  const following = !paused && loop.kind !== 'stopped'
  const unseen = pinned ? 0 : unseenLines(tail.nextSeq, seenSeq)
  const gutterDigits = String(Math.max(1, tail.nextSeq)).length

  // Stay on the newest line while pinned.
  useLayoutEffect(() => {
    const pane = paneRef.current
    if (pane && pinned) {
      pane.scrollTop = pane.scrollHeight
      setSeenSeq(tail.nextSeq)
    }
  }, [tail.nextSeq, pinned])

  const toBottom = useCallback(() => {
    const pane = paneRef.current
    if (pane) pane.scrollTop = pane.scrollHeight
    setPinned(true)
    setSeenSeq(tail.nextSeq)
  }, [tail.nextSeq])

  const onScroll = () => {
    const pane = paneRef.current
    if (!pane) return
    const near = isNearBottom(pane)
    setPinned(near)
    if (near) setSeenSeq(tail.nextSeq)
  }

  const togglePause = () => {
    if (paused) resume()
    else pause()
  }

  const onPaneKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const action = keyToAction(e.key, e.ctrlKey || e.metaKey || e.altKey)
    if (action === null) return
    e.preventDefault()
    if (action === 'toggle') togglePause()
    else toBottom()
  }

  const copy = () => {
    const text = lines.map((l) => l.text).join('\n')
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setNotice(`copied ${lines.length.toLocaleString('en-US')} ${lines.length === 1 ? 'line' : 'lines'}`)
      })
      .catch(() => {
        setNotice('could not copy')
      })
    window.setTimeout(() => {
      setNotice(null)
    }, 2500)
  }

  const note = loopNote(loop, paused)

  return (
    <div
      className={`flex flex-col overflow-hidden rounded-xl border border-white/10 bg-[#0b0f14] font-mono text-gray-200 shadow-lg ${className}`}
    >
      {/* title bar */}
      <div className="flex items-center gap-3 border-b border-white/10 bg-[#11161d] px-4 py-2">
        <span className="flex gap-1.5" aria-hidden="true">
          <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
          <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
          <span className="h-3 w-3 rounded-full bg-[#28c840]" />
        </span>
        <span className="min-w-0 flex-1 truncate text-center text-xs text-[#8b95a5]" title={name}>
          {name}
        </span>
        <span
          className={`flex items-center gap-1.5 rounded px-1.5 py-0.5 text-[11px] font-semibold tracking-wide ${
            following ? 'bg-red-500/15 text-red-300' : 'bg-white/10 text-gray-400'
          }`}
        >
          <span
            aria-hidden="true"
            className={`h-1.5 w-1.5 rounded-full ${following ? 'bg-red-400 motion-safe:animate-pulse' : 'bg-gray-500'}`}
          />
          {following ? 'LIVE' : paused ? 'PAUSED' : 'STOPPED'}
        </span>
      </div>

      {/* tools */}
      <div className="flex flex-wrap items-center gap-1 border-b border-white/10 bg-[#0e131a] px-3 py-1.5">
        <button
          type="button"
          className={`${buttonClass} ${paused ? 'bg-amber-400/20 text-amber-200' : ''}`}
          aria-pressed={paused}
          onClick={togglePause}
          disabled={loop.kind === 'stopped'}
          title="Pause or resume (F)"
        >
          {paused ? '▶ Resume' : '⏸ Pause'}
        </button>
        <button type="button" className={buttonClass} onClick={toBottom} disabled={pinned} title="Jump to the newest line (End or G)">
          ⏭ Latest
        </button>
        <button
          type="button"
          aria-pressed={wrap}
          className={`${buttonClass} ${wrap ? 'bg-emerald-400/20 text-emerald-200' : ''}`}
          onClick={() => {
            setWrap((w) => !w)
          }}
          title="Wrap long lines"
        >
          Wrap
        </button>
        <button type="button" className={buttonClass} onClick={copy} disabled={lines.length === 0}>
          Copy visible
        </button>
        <button
          type="button"
          className={`${buttonClass} ml-auto`}
          onClick={restart}
          title="Forget what is held and follow from the end again"
        >
          Start over
        </button>
      </div>

      {loop.kind === 'stopped' ? (
        <div role="alert" className={`flex ${paneHeight} flex-col items-center justify-center gap-3 px-6 text-center text-sm text-red-300`}>
          <p>
            <span aria-hidden="true">✗ </span>
            {loop.message}
          </p>
          <button type="button" className={`${buttonClass} border border-white/20`} onClick={restart}>
            Try again
          </button>
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1">
          {/* A scrollable text region must be reachable by keyboard (so it can be scrolled and paused), which these two rules cannot express. */}
          {/* eslint-disable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/no-noninteractive-tabindex */}
          <div
            ref={paneRef}
            role="region"
            aria-label={`${name} live contents`}
            tabIndex={0}
            onKeyDown={onPaneKey}
            onScroll={onScroll}
            className={`${paneHeight} min-w-0 flex-1 overflow-auto py-2 text-[13px] leading-5 outline-none selection:bg-emerald-400/30 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-emerald-400`}
          >
            {lines.length === 0 && (
              <p className="px-4 py-8 text-center text-[#8b95a5]">{paused ? 'Paused. Nothing new is shown.' : 'Waiting for new lines…'}</p>
            )}
            {lines.map((l) => {
              const kind = highlight?.(l.text) ?? null
              return (
                <div key={l.seq} className={`flex ${wrap ? '' : 'min-w-max'} hover:bg-white/[0.03]`}>
                  <span
                    style={{ width: `${gutterDigits + 2}ch` }}
                    className="sticky left-0 shrink-0 select-none bg-[#0b0f14] pr-3 text-right text-[#8b95a5]"
                    title="Counted from when you started following"
                  >
                    {lineNumber(l)}
                  </span>
                  <span
                    className={`pr-4 ${wrap ? 'min-w-0 whitespace-pre-wrap break-words' : 'whitespace-pre'} ${kind ? HIGHLIGHT_CLASS[kind] : ''}`}
                  >
                    {l.text}
                    {l.truncated && (
                      <span className="ml-2 select-none text-amber-300" title={`${l.bytes.toLocaleString('en-US')} bytes in all`}>
                        … cut
                      </span>
                    )}
                  </span>
                </div>
              )
            })}
          </div>
          {/* eslint-enable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/no-noninteractive-tabindex */}
          {!pinned && (
            <button
              type="button"
              onClick={toBottom}
              className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border border-emerald-400/40 bg-[#10241d] px-3 py-1 text-xs text-emerald-200 shadow-lg hover:bg-[#143127] focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400"
            >
              {jumpLabel(unseen)}
            </button>
          )}
        </div>
      )}

      {/* prompt line: what is held, in terminal words */}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-white/10 bg-[#11161d] px-4 py-2 text-xs">
        <span className="min-w-0 truncate text-emerald-400">
          $ tail -f {name}
          {following && <span className="cinqo-caret ml-1 inline-block h-3.5 w-1.5 translate-y-0.5 bg-emerald-400" aria-hidden="true" />}
        </span>
        <span className={notice ? 'text-emerald-300' : note ? 'text-amber-300' : 'text-[#8b95a5]'}>
          {notice ?? note ?? statusText(tail, following)}
        </span>
      </div>
      {/* what a screen reader hears: the state, never a line at a time */}
      <div className="sr-only" aria-live="polite">
        {announce(loop, paused)}
      </div>
    </div>
  )
}
