import { useEffect, useRef, useState } from 'react'
import { THINKING_WORDS } from './thinkingWords'
import { formatDuration } from './formatDuration'
import { effectiveStartMs } from './turnTiming'

function randomWord(): string {
  return THINKING_WORDS[Math.floor(Math.random() * THINKING_WORDS.length)]
}

// STILL_THINKING_AT_MS/STILL_THINKING_HIDE_AT_MS/TAKING_LONGER_AT_MS
// (step — ThinkingIndicator polish) — long-wait reassurance text,
// purely derived from elapsedMs (already ticking once/second below),
// no separate timer needed.
const STILL_THINKING_AT_MS = 60_000
const STILL_THINKING_HIDE_AT_MS = 70_000
const TAKING_LONGER_AT_MS = 180_000

// A small, local, self-contained icon — no robot glyph exists in this
// app's own icon sets (Shared/Components/IconButton/icons.tsx is
// scoped to IconButton's own actions), matching the tool frontends'
// own convention of a one-off local SVG rather than a shared
// dependency for a single use site.
function RobotIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4 shrink-0">
      <rect x="4" y="7" width="12" height="9" rx="2" strokeWidth="1.4" />
      <path d="M10 7V4" strokeWidth="1.4" strokeLinecap="round" />
      <circle cx="10" cy="2.8" r="1" fill="currentColor" stroke="none" />
      <circle cx="7.5" cy="11.5" r="1" fill="currentColor" stroke="none" />
      <circle cx="12.5" cy="11.5" r="1" fill="currentColor" stroke="none" />
      <path d="M7 14.5h6" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M2 10h2M16 10h2" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

// Claude-Code-style "thinking" placeholder — a random word (re-rolled
// every 15s, step 21, with a per-letter wave animation on change,
// step 22) plus animated dots and a live elapsed timer, replacing the
// previous static "Thinking…" text. Rendered by MessageThread.tsx
// only while `sending` is true.
//
// startedAt/clockOffsetMs come from the backend (turn_runs.started_at,
// step 23), not a locally-captured mount timestamp — a turn resumed
// after reopening the page (or after this component simply remounts)
// must show real elapsed time immediately, not restart from 0s. See
// plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md,
// which supersedes this component's original Date.now()-at-mount
// design (step-21-thinking-indicator-timer.md).
export function ThinkingIndicator({ startedAt, clockOffsetMs }: { startedAt: string; clockOffsetMs: number }) {
  const startRef = useRef(effectiveStartMs(startedAt, clockOffsetMs))
  const wordSlotRef = useRef(0)
  const [word, setWord] = useState(() => randomWord())
  const [dotCount, setDotCount] = useState(0)
  // Computed eagerly (not 0) so a resumed turn shows its real elapsed
  // time on the very first render, never a visible "0s" flash before
  // the first tick corrects it.
  const [elapsedMs, setElapsedMs] = useState(() => Math.max(0, Date.now() - startRef.current))

  // Kept in sync with the latest server-derived baseline — a later
  // poll response (ConversationContext) can refine clockOffsetMs
  // slightly over a long-running turn; harmless, imperceptible
  // corrections rather than a value fixed forever at first render.
  useEffect(() => {
    startRef.current = effectiveStartMs(startedAt, clockOffsetMs)
  }, [startedAt, clockOffsetMs])

  // Dots — its own fast interval, a lively "typing" feel; unrelated to
  // the once-a-second timer below.
  useEffect(() => {
    const id = setInterval(() => {
      setDotCount((d) => (d + 1) % 4)
    }, 400)
    return () => {
      clearInterval(id)
    }
  }, [])

  // Elapsed seconds + 15s word rotation — a separate, once-a-second
  // interval (step 22 split this out of the old shared 400ms one) so
  // the displayed number visibly increases by exactly 1 each tick,
  // not 2.5 times a second. Still computed from the real start time
  // each tick (Math.floor, not a naive +1 accumulator), so it can't
  // drift under load — it just happens to equal "+1 from last tick"
  // under normal conditions.
  useEffect(() => {
    const id = setInterval(() => {
      const elapsedSeconds = Math.floor(Math.max(0, Date.now() - startRef.current) / 1000)
      setElapsedMs(elapsedSeconds * 1000)

      const slot = Math.floor(elapsedSeconds / 15)
      if (slot !== wordSlotRef.current) {
        wordSlotRef.current = slot
        setWord(randomWord())
      }
    }, 1000)
    return () => {
      clearInterval(id)
    }
  }, [])

  const showStillThinking = elapsedMs >= STILL_THINKING_AT_MS && elapsedMs < STILL_THINKING_HIDE_AT_MS
  const showTakingLonger = elapsedMs >= TAKING_LONGER_AT_MS

  return (
    <div className="max-w-lg text-sm text-gray-400">
      <div className="flex items-center">
        <RobotIcon />
        {/* key={word} forces a fresh mount of the letter spans below on
            every word change, so the pop-in wave replays each time —
            React only (re-)runs a CSS animation on a freshly mounted
            element. The whole word also shimmers continuously
            (thinking-shimmer, index.css) between the normal and a
            lighter shade, independent of the word itself changing —
            a lightweight "something is happening" pulse. */}
        <span key={word} className="ml-1.5 inline-flex animate-[thinking-shimmer_3.5s_ease-in-out_infinite]">
          {word.split('').map((ch, i) => (
            <span
              key={i}
              className="inline-block animate-[thinking-wave_0.4s_ease]"
              style={{ animationDelay: `${i * 30}ms`, animationFillMode: 'backwards' }}
            >
              {ch}
            </span>
          ))}
        </span>
        {/* Always reserves 3 characters of width (real dots up to
            dotCount, the rest invisible) so the row doesn't visibly
            resize 2.5 times a second as the dot count cycles. */}
        <span aria-hidden="true">
          {'.'.repeat(dotCount)}
          <span className="invisible">{'.'.repeat(3 - dotCount)}</span>
        </span>
        {/* Skipped for the first second — a near-instant reply doesn't
            need a "(0.3s)" flash before the row disappears. */}
        {elapsedMs >= 1000 && <span className="ml-1.5 tabular-nums text-gray-400">({formatDuration(elapsedMs)})</span>}
      </div>
      {/* Long-wait reassurance text — "Still thinking" auto-hides again
          10s after it appears (showStillThinking's own derivation),
          "This is taking a little bit longer" persists once shown
          (no upper bound), never both at once. */}
      {showStillThinking && (
        <div className="mt-1 animate-[thinking-fade-in_0.3s_ease] pl-[1.375rem] text-xs text-gray-400">Still thinking…</div>
      )}
      {showTakingLonger && (
        <div className="mt-1 animate-[thinking-fade-in_0.3s_ease] pl-[1.375rem] text-xs text-gray-400">
          This is taking a little bit longer…
        </div>
      )}
    </div>
  )
}
