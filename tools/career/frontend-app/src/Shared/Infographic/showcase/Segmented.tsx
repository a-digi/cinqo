import { useRef, useState } from 'react'
import { toneColors, type ShowcaseTone } from './showcaseTokens'

export interface SegmentedTab {
  id: string
  label: string
  panel: React.ReactNode
}

// A row of pill tabs that swaps one panel (the showcase look's segmented control). It is a real tab list: the arrow keys move between the tabs, Home and End go to the first and the
// last, exactly one panel is shown, and each tab names its panel.
export function Segmented({ idPrefix, tabs, tone, label }: { idPrefix: string; tabs: SegmentedTab[]; tone: ShowcaseTone; label: string }) {
  const [active, setActive] = useState(0)
  const buttons = useRef<(HTMLButtonElement | null)[]>([])
  const c = toneColors(tone)
  const dark = tone === 'dark'
  function move(to: number) {
    const next = (to + tabs.length) % tabs.length
    setActive(next)
    buttons.current[next]?.focus()
  }
  return (
    <div>
      <div
        role="tablist"
        aria-label={label}
        className="mx-auto inline-flex rounded-full p-1"
        style={{ background: dark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.06)' }}
      >
        {tabs.map((t, i) => {
          const on = i === active
          return (
            <button
              key={t.id}
              ref={(el) => {
                buttons.current[i] = el
              }}
              type="button"
              role="tab"
              id={`${idPrefix}-${t.id}-tab`}
              aria-selected={on}
              aria-controls={`${idPrefix}-${t.id}-panel`}
              tabIndex={on ? 0 : -1}
              onClick={() => {
                setActive(i)
              }}
              onKeyDown={(e) => {
                if (e.key === 'ArrowRight') {
                  e.preventDefault()
                  move(active + 1)
                } else if (e.key === 'ArrowLeft') {
                  e.preventDefault()
                  move(active - 1)
                } else if (e.key === 'Home') {
                  e.preventDefault()
                  move(0)
                } else if (e.key === 'End') {
                  e.preventDefault()
                  move(tabs.length - 1)
                }
              }}
              className="rounded-full px-5 py-2 text-sm font-medium transition-colors motion-reduce:transition-none md:text-base"
              style={{
                background: on ? (dark ? '#ffffff' : '#1d1d1f') : 'transparent',
                color: on ? (dark ? '#000000' : '#ffffff') : c.color,
              }}
            >
              {t.label}
            </button>
          )
        })}
      </div>
      {tabs.map((t, i) => (
        <div
          key={t.id}
          role="tabpanel"
          id={`${idPrefix}-${t.id}-panel`}
          aria-labelledby={`${idPrefix}-${t.id}-tab`}
          hidden={i !== active}
          className="showcase-panel mt-12"
        >
          {i === active ? t.panel : null}
        </div>
      ))}
    </div>
  )
}
