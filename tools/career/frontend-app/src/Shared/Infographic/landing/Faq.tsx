import { useState } from 'react'
import { LANDING } from './landingTokens'

export interface FaqItem {
  id: string
  question: string
  answer: string
}

// A question section: heading and an accordion (the landing look's "questions" block). Each question is a button that opens its answer; several can be open
// at once; the first can start open.
export function Faq({
  idPrefix,
  title,
  items,
  startOpen = null,
}: {
  idPrefix: string
  title: string
  items: FaqItem[]
  startOpen?: string | null
}) {
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set(startOpen === null ? [] : [startOpen]))
  const toggle = (id: string) => {
    setOpen((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  return (
    <div className="mx-auto max-w-3xl" data-testid="faq">
      <h3 className="mb-6 text-center text-3xl font-extrabold tracking-tight md:text-4xl" style={{ color: LANDING.ink }}>
        {title}
      </h3>
      <ul className="divide-y divide-gray-300 border-y border-gray-300">
        {items.map((it) => {
          const isOpen = open.has(it.id)
          return (
            <li key={it.id}>
              <h4>
                <button
                  type="button"
                  id={`${idPrefix}-${it.id}-q`}
                  aria-expanded={isOpen}
                  aria-controls={`${idPrefix}-${it.id}-a`}
                  onClick={() => {
                    toggle(it.id)
                  }}
                  className="flex w-full items-center justify-between gap-4 py-5 text-left text-base font-bold md:text-lg"
                  style={{ color: LANDING.ink }}
                >
                  {it.question}
                  <span
                    aria-hidden="true"
                    className="shrink-0 text-xl transition-transform duration-200 motion-reduce:transition-none"
                    style={{ transform: isOpen ? 'rotate(180deg)' : 'none' }}
                  >
                    ⌄
                  </span>
                </button>
              </h4>
              <div
                id={`${idPrefix}-${it.id}-a`}
                role="region"
                aria-labelledby={`${idPrefix}-${it.id}-q`}
                hidden={!isOpen}
                className="pb-5 pr-8"
              >
                <p className="text-base leading-relaxed" style={{ color: LANDING.muted }}>
                  {it.answer}
                </p>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
