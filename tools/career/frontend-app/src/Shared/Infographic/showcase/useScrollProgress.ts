import { useEffect, useRef, useState } from 'react'
import { prefersReducedMotion, scrollParentOf } from '../scrollRules'
import { pinnedProgress } from './pinnedRules'

// How far the person has scrolled through a tall pinned section (0 to 1), read from the scroller that holds it (the body of the full-page modal). It looks again on every scroll, at most
// once per frame, and when the window is resized. `stageOffset` is where the sticky stage sticks (its `top` in px). Returns 0 until the first look.
export function useScrollProgress<T extends HTMLElement>(stageOffset: number): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null)
  const [progress, setProgress] = useState(0)
  useEffect(() => {
    const el = ref.current
    const scroller = scrollParentOf(el)
    if (!el || !scroller) return
    let frame = 0
    const look = () => {
      frame = 0
      const top = el.getBoundingClientRect().top - scroller.getBoundingClientRect().top
      const stage =
        el.firstElementChild instanceof HTMLElement ? el.firstElementChild.getBoundingClientRect().height : scroller.clientHeight
      setProgress(pinnedProgress(top, el.getBoundingClientRect().height, stage, stageOffset))
    }
    const onScroll = () => {
      if (frame === 0) frame = requestAnimationFrame(look)
    }
    look()
    scroller.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      scroller.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      if (frame !== 0) cancelAnimationFrame(frame)
    }
  }, [stageOffset])
  return [ref, progress]
}

// Whether the pinned stage is used at all: a wide window and no wish for less motion (otherwise its steps are a plain list).
export function usePinnedMode(): boolean {
  const [pinned, setPinned] = useState(() => decide())
  useEffect(() => {
    const update = () => {
      setPinned(decide())
    }
    window.addEventListener('resize', update)
    return () => {
      window.removeEventListener('resize', update)
    }
  }, [])
  return pinned
}

function decide(): boolean {
  if (prefersReducedMotion()) return false
  // Where the browser cannot tell the width (some test environments) the wide layout is used.
  return typeof window.matchMedia !== 'function' || window.matchMedia('(min-width: 768px)').matches
}
