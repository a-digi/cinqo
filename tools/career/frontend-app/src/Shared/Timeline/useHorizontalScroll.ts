import { useCallback, useEffect, useRef, useState } from 'react'
import { dragTarget, isDrag, pageTarget, scrollEdges, scrollProgress, thumbFraction, type ScrollEdges } from './scrollRules'

export interface HorizontalScrollState {
  edges: ScrollEdges
  progress: number
  thumb: number
}

const IDLE: HorizontalScrollState = { edges: { canLeft: false, canRight: false }, progress: 0, thumb: 1 }

// The behaviour of a sideways scrolling area (plan/ai/tools/career/step-170-onboarding-guide.md): it follows the scroll position and the size
// (for the edge fade, the progress thumb and the buttons), scrolls a page at a time on request, and lets a mouse drag the content. The
// rules are in scrollRules.ts. Touch and trackpad scroll natively, so only a mouse needs the drag.
export function useHorizontalScroll() {
  const ref = useRef<HTMLDivElement>(null)
  const [state, setState] = useState<HorizontalScrollState>(IDLE)
  const [dragging, setDragging] = useState(false)
  const press = useRef<{ x: number; left: number; id: number } | null>(null)
  const dragged = useRef(false)

  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const next: HorizontalScrollState = {
      edges: scrollEdges(el.scrollLeft, el.clientWidth, el.scrollWidth),
      progress: scrollProgress(el.scrollLeft, el.clientWidth, el.scrollWidth),
      thumb: thumbFraction(el.clientWidth, el.scrollWidth),
    }
    setState((prev) =>
      prev.edges.canLeft === next.edges.canLeft &&
      prev.edges.canRight === next.edges.canRight &&
      Math.abs(prev.progress - next.progress) < 0.002 &&
      Math.abs(prev.thumb - next.thumb) < 0.002
        ? prev
        : next,
    )
  }, [])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    let frame = 0
    const onScroll = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    measure()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(el)
    if (el.firstElementChild) observer?.observe(el.firstElementChild)
    return () => {
      cancelAnimationFrame(frame)
      el.removeEventListener('scroll', onScroll)
      observer?.disconnect()
    }
  }, [measure])

  const page = useCallback((direction: -1 | 1) => {
    const el = ref.current
    if (!el) return
    el.scrollTo({ left: pageTarget(el.scrollLeft, el.clientWidth, el.scrollWidth, direction), behavior: 'smooth' })
  }, [])

  const dragProps = {
    onPointerDown: (e: React.PointerEvent<HTMLDivElement>) => {
      if (e.pointerType !== 'mouse' || e.button !== 0 || !ref.current) return
      press.current = { x: e.clientX, left: ref.current.scrollLeft, id: e.pointerId }
      dragged.current = false
    },
    onPointerMove: (e: React.PointerEvent<HTMLDivElement>) => {
      const start = press.current
      const el = ref.current
      if (!start || !el) return
      if (!dragged.current) {
        if (!isDrag(start.x, e.clientX)) return
        // Only now take the pointer: capturing at the press would turn a plain click on a link into a click on this area.
        dragged.current = true
        el.setPointerCapture(start.id)
        setDragging(true)
      }
      el.scrollLeft = dragTarget(start.left, start.x, e.clientX, el.clientWidth, el.scrollWidth)
    },
    onPointerUp: () => {
      const el = ref.current
      if (press.current && dragged.current && el?.hasPointerCapture(press.current.id)) el.releasePointerCapture(press.current.id)
      press.current = null
      setDragging(false)
    },
    onPointerCancel: () => {
      press.current = null
      dragged.current = false
      setDragging(false)
    },
    // The click that ends a drag must not follow the link the pointer happens to be over.
    onClickCapture: (e: React.MouseEvent<HTMLDivElement>) => {
      if (dragged.current) {
        e.preventDefault()
        e.stopPropagation()
        dragged.current = false
      }
    },
  }

  return { ref, state, dragging, page, dragProps }
}
