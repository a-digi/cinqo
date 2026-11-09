import { useEffect, useRef, useState } from 'react'
import type { SystemResources } from '../api/system'

// The numbers of the bar glide to a new reading instead of jumping, and a figure that changed lights up for a moment. Both are switched
// off for a person who asked their system for less motion (prefers-reduced-motion).
export const TWEEN_MS = 700
export const FLASH_MS = 900

export function prefersReducedMotion(): boolean {
  try {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches
  } catch {
    return false
  }
}

export function easeOut(t: number): number {
  const c = Math.min(1, Math.max(0, t))
  return 1 - Math.pow(1 - c, 3)
}

function mix(a: number, b: number, t: number): number {
  return Math.round(a + (b - a) * t)
}

// The reading between two readings: every number moves by the eased fraction t (0 = from, 1 = to); everything else is the new reading's.
// A reading from a computer that reports no value (-1 load, 0 bytes) is never mixed with a real one: it jumps.
export function mixResources(from: SystemResources, to: SystemResources, t: number): SystemResources {
  const e = easeOut(t)
  const m = (a: number, b: number) => (a <= 0 || b <= 0 ? b : mix(a, b, e))
  return {
    ...to,
    memory: {
      ...to.memory,
      totalBytes: to.memory.totalBytes,
      availableBytes: m(from.memory.availableBytes, to.memory.availableBytes),
      swapUsedBytes: mix(from.memory.swapUsedBytes, to.memory.swapUsedBytes, e),
    },
    cinqo: {
      appBytes: m(from.cinqo.appBytes, to.cinqo.appBytes),
      localAiBytes: mix(from.cinqo.localAiBytes, to.cinqo.localAiBytes, e),
    },
    cpu: {
      ...to.cpu,
      load1:
        from.cpu.load1 < 0 || to.cpu.load1 < 0
          ? to.cpu.load1
          : Math.round((from.cpu.load1 + (to.cpu.load1 - from.cpu.load1) * e) * 10) / 10,
    },
    disk: { ...to.disk, freeBytes: m(from.disk.freeBytes, to.disk.freeBytes) },
    agents: to.agents.map((a) => {
      const before = from.agents.find((x) => x.kind === a.kind)
      return before ? { ...a, memoryBytes: m(before.memoryBytes, a.memoryBytes) } : a
    }),
  }
}

// The reading to show: it glides from what is shown now to the newest reading over TWEEN_MS. The first reading is shown at once.
export function useTweenedResources(target: SystemResources | null): SystemResources | null {
  const [shown, setShown] = useState<SystemResources | null>(target)
  const shownRef = useRef<SystemResources | null>(target)
  const raf = useRef<number | undefined>(undefined)

  useEffect(() => {
    if (raf.current !== undefined) cancelAnimationFrame(raf.current)
    if (target === null) {
      shownRef.current = null
      setShown(null)
      return
    }
    const from = shownRef.current
    if (from === null || prefersReducedMotion()) {
      shownRef.current = target
      setShown(target)
      return
    }
    const start = performance.now()
    const step = (now: number) => {
      const t = (now - start) / TWEEN_MS
      const next = t >= 1 ? target : mixResources(from, target, t)
      shownRef.current = next
      setShown(next)
      if (t < 1) raf.current = requestAnimationFrame(step)
    }
    raf.current = requestAnimationFrame(step)
    return () => {
      if (raf.current !== undefined) cancelAnimationFrame(raf.current)
    }
  }, [target])

  return shown
}

// true for FLASH_MS after the value changed (never on the first render).
export function useFlash(value: string): boolean {
  const [on, setOn] = useState(false)
  const last = useRef(value)
  useEffect(() => {
    if (last.current === value) return
    last.current = value
    if (prefersReducedMotion()) return
    setOn(true)
    const t = setTimeout(() => {
      setOn(false)
    }, FLASH_MS)
    return () => {
      clearTimeout(t)
    }
  }, [value])
  return on
}
