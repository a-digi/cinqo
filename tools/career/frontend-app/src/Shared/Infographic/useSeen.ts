import { useEffect, useRef, useState } from 'react'

// True once the element has come into view (and stays true): the one watcher both Reveal and the arrows use. Where the browser cannot tell (no
// IntersectionObserver) it is true from the start.
export function useSeen<T extends HTMLElement>(): [React.RefObject<T | null>, boolean] {
  const ref = useRef<T>(null)
  const [shown, setShown] = useState(() => typeof IntersectionObserver === 'undefined')
  useEffect(() => {
    if (shown) return
    const el = ref.current
    if (!el) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setShown(true)
          io.disconnect()
        }
      },
      { threshold: 0.12 },
    )
    io.observe(el)
    return () => {
      io.disconnect()
    }
  }, [shown])
  return [ref, shown]
}
