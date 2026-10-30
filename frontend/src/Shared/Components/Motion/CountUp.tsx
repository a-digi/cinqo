// The same count-up as the Career tool's (Shared/Motion/CountUp.tsx); the app and a tool bundle are built apart, so each keeps its own copy.
import { useEffect, useRef, useState } from 'react'
import { easeOut, formatCount, parseCount } from './countRules'

// A figure that counts up to its value when it appears (and from its old value to a new one when it
// changes). Text with no number in it, and people who ask their system for less motion, get the
// final text at once. The screen reader always reads the final text.
export function CountUp({ value, durationMs = 1300, delayMs = 0 }: { value: string; durationMs?: number; delayMs?: number }) {
  const parsed = parseCount(value)
  const [shown, setShown] = useState(() => (parsed ? formatCount(parsed, 0) : value))
  const from = useRef(0)

  useEffect(() => {
    const target = parseCount(value)
    if (!target || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      setShown(value)
      return
    }
    const start = from.current
    let frame = 0
    let timer = 0
    let t0 = 0
    const step = (now: number) => {
      if (t0 === 0) t0 = now
      const progress = (now - t0) / durationMs
      const current = start + (target.value - start) * easeOut(progress)
      setShown(formatCount(target, progress >= 1 ? target.value : current))
      if (progress < 1) frame = requestAnimationFrame(step)
      else from.current = target.value
    }
    timer = window.setTimeout(() => {
      frame = requestAnimationFrame(step)
    }, delayMs)
    return () => {
      window.clearTimeout(timer)
      cancelAnimationFrame(frame)
    }
  }, [value, durationMs, delayMs])

  return (
    <span aria-label={value}>
      <span aria-hidden="true">{shown}</span>
    </span>
  )
}
