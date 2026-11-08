import { useRef } from 'react'
import { swipeDirection, type Direction } from './sliderRules'

// Pointer handlers that turn a horizontal drag into onSwipe('next' | 'prev'). Put them on the element
// that holds the slides, with `touch-action: pan-y` so a vertical swipe still scrolls the page.
export function useSwipe(onSwipe: (direction: Direction) => void, threshold = 50) {
  const start = useRef<{ x: number; y: number } | null>(null)
  return {
    onPointerDown: (e: React.PointerEvent) => {
      // Only the primary button (or a finger); a right click is not a drag.
      if (e.pointerType === 'mouse' && e.button !== 0) return
      start.current = { x: e.clientX, y: e.clientY }
    },
    onPointerUp: (e: React.PointerEvent) => {
      const from = start.current
      start.current = null
      if (!from) return
      const direction = swipeDirection(e.clientX - from.x, e.clientY - from.y, threshold)
      if (direction) onSwipe(direction)
    },
    onPointerCancel: () => {
      start.current = null
    },
  }
}
