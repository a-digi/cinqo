import { useEffect, useState } from 'react'

// The width from which the instruction modal puts its help beside the instructions (plan/ai/tools/career/step-246-instruction-modal-two-columns-and-design.md).
export const WIDE_QUERY = '(min-width: 1024px)'

function matches(): boolean {
  return typeof window.matchMedia === 'function' ? window.matchMedia(WIDE_QUERY).matches : true
}

// Whether the window is at least 1024 px wide, following the window as it is resized. Where the browser cannot say (no matchMedia), the wide layout is used.
export function useIsWide(): boolean {
  const [wide, setWide] = useState(matches)
  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const query = window.matchMedia(WIDE_QUERY)
    const onChange = () => {
      setWide(query.matches)
    }
    onChange()
    query.addEventListener('change', onChange)
    return () => {
      query.removeEventListener('change', onChange)
    }
  }, [])
  return wide
}
