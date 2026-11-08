import { useEffect, useRef, useState } from 'react'

const DEFAULT_DURATION_MS = 500

// useAnimatedNumber smoothly counts from whatever value is currently
// displayed up to `target`, instead of jumping straight to it —
// ThinkingIndicator's own live token counters only ever increase (real
// usage accumulates, never decreases), so every 2s poll reads as a
// gentle "counting up" tick rather than a jarring instant jump. Eased
// (decelerating), not linear — matches how a real running total feels.
// See plan/ai/conversation/step-34-realtime-token-usage-budget-and-display.md.
export function useAnimatedNumber(target: number, durationMs = DEFAULT_DURATION_MS): number {
  const [displayed, setDisplayed] = useState(target)
  const startRef = useRef<number | null>(null)
  const frameRef = useRef<number | null>(null)

  useEffect(() => {
    const from = displayed
    const delta = target - from
    if (delta === 0) return

    startRef.current = null
    function tick(now: number) {
      startRef.current ??= now
      const progress = Math.min(1, (now - startRef.current) / durationMs)
      const eased = 1 - Math.pow(1 - progress, 3)
      setDisplayed(Math.round(from + delta * eased))
      if (progress < 1) {
        frameRef.current = requestAnimationFrame(tick)
      }
    }
    frameRef.current = requestAnimationFrame(tick)

    return () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current)
    }
    // Deliberately NOT depending on `displayed` — this effect must
    // only (re-)start when `target` itself changes; including
    // `displayed` would restart the animation on every one of its own
    // in-flight setDisplayed calls, never letting it finish.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, durationMs])

  return displayed
}
