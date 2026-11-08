import { useEffect, useState, type RefObject } from 'react'

// Which items have scrolled into view (plan/ai/tools/career/step-170-onboarding-guide.md): `ids` are the data-step-id of the items inside the
// scrolling area `root`. Once seen an item stays seen, so scrolling back does not replay the effect. Without IntersectionObserver, or for a
// person who asked for less motion, everything counts as seen at once.
export function useReveal(root: RefObject<HTMLElement | null>, ids: readonly string[]): ReadonlySet<string> {
  const [seen, setSeen] = useState<ReadonlySet<string>>(new Set())
  const key = ids.join('\u0000')
  useEffect(() => {
    const area = root.current
    if (!area) return
    const everything = () => {
      setSeen(new Set(ids))
    }
    const reduced = typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches
    if (reduced || typeof IntersectionObserver === 'undefined') {
      everything()
      return
    }
    const observer = new IntersectionObserver(
      (entries) => {
        const fresh = entries.filter((entry) => entry.isIntersecting).map((entry) => (entry.target as HTMLElement).dataset.stepId ?? '')
        if (fresh.length === 0) return
        setSeen((prev) => {
          const next = new Set(prev)
          for (const id of fresh) if (id !== '') next.add(id)
          return next.size === prev.size ? prev : next
        })
      },
      { root: area, threshold: 0.2 },
    )
    area.querySelectorAll('[data-step-id]').forEach((el) => {
      observer.observe(el)
    })
    return () => {
      observer.disconnect()
    }
    // `key` stands for the list of ids.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [root, key])
  return seen
}
