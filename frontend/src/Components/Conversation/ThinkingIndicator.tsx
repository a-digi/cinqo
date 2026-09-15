import { useEffect, useRef, useState } from 'react'
import { THINKING_WORDS } from './thinkingWords'
import { formatDuration } from './formatDuration'

function randomWord(): string {
  return THINKING_WORDS[Math.floor(Math.random() * THINKING_WORDS.length)]
}

// effectiveStartMs is startedAt (RFC3339, the server's own recorded
// turn-start time) shifted by clockOffsetMs (serverNow - Date.now(),
// as of the most recent poll — see ConversationContext's own
// pollUntilFinished) so that Date.now() - effectiveStartMs always
// equals real server-side elapsed time, even when the viewer's local
// clock disagrees with the server's.
function effectiveStartMs(startedAt: string, clockOffsetMs: number): number {
  return Date.parse(startedAt) - clockOffsetMs
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
    const id = setInterval(() => setDotCount((d) => (d + 1) % 4), 400)
    return () => clearInterval(id)
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
    return () => clearInterval(id)
  }, [])

  return (
    <div className="max-w-lg rounded-lg bg-gray-100 px-3 py-2 text-sm text-gray-400">
      {/* key={word} forces a fresh mount of the letter spans below on
          every word change, so the wave animation replays each time —
          React only (re-)runs a CSS animation on a freshly mounted
          element. */}
      <span key={word} className="inline-flex">
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
          dotCount, the rest invisible) so the bubble doesn't visibly
          resize 2.5 times a second as the dot count cycles. */}
      <span aria-hidden="true">
        {'.'.repeat(dotCount)}
        <span className="invisible">{'.'.repeat(3 - dotCount)}</span>
      </span>
      {/* Skipped for the first second — a near-instant reply doesn't
          need a "(0.3s)" flash before the bubble disappears. */}
      {elapsedMs >= 1000 && <span className="ml-1.5 tabular-nums text-gray-400">({formatDuration(elapsedMs)})</span>}
    </div>
  )
}
