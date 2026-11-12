import { useCallback, useEffect, useRef, useState } from 'react'
import { Modal } from '../../Modal/Modal'
import { chapterInView } from '../infographicRules'
import { BAR_LINE, prefersReducedMotion, scrollParentOf } from '../scrollRules'
import { IconTabs, type IconTab } from './IconTabs'
import { LANDING } from './landingTokens'

// A section that a tab click scrolled to sits at the scroll margin (96 px) give or take a fraction of a pixel (96.19 was seen live); this much of slack
// keeps the tab that was clicked the current one.
export const SETTLE = 8

export interface LandingSection {
  id: string
  // The full heading of the section: the region is named by it.
  title: string
}

export interface LandingProps {
  open: boolean
  onClose: () => void
  // The title of the full-page modal (its own header, with the Close button, stays above the hero).
  title: string
  // The start of every id ("profile-info": "profile-info-roles").
  idPrefix: string
  // The hero band, drawn first; it gets `go` to scroll to a section.
  hero: (go: (id: string) => void) => React.ReactNode
  // One tab and one section per chapter, in order.
  tabs: IconTab[]
  sections: LandingSection[]
  // The content of a section (tiles, an accordion, cards).
  renderSection: (id: string, go: (id: string) => void) => React.ReactNode
}

// A "How it works" page in the landing look (plan 267): a navy hero, a sticky strip of icon tabs that marks the chapter in view and scrolls to one on a
// click, then the sections one under the other on a pale page. The content of every part is given; this only lays it out and follows the scroll.
export function LandingModal(props: LandingProps) {
  return (
    <Modal open={props.open} title={props.title} onClose={props.onClose} size="full">
      <Page {...props} />
    </Modal>
  )
}

function Page({ idPrefix, hero, tabs, sections, renderSection }: LandingProps) {
  const root = useRef<HTMLDivElement>(null)
  const refs = useRef<Record<string, HTMLElement | null>>({})
  const [current, setCurrent] = useState(0)

  useEffect(() => {
    const scroller = scrollParentOf(root.current)
    if (!scroller) return
    const update = () => {
      const top = scroller.getBoundingClientRect().top
      const tops = sections.map((s) => (refs.current[s.id]?.getBoundingClientRect().top ?? Number.POSITIVE_INFINITY) - top)
      setCurrent(chapterInView(tops, BAR_LINE + SETTLE))
    }
    update()
    scroller.addEventListener('scroll', update, { passive: true })
    return () => {
      scroller.removeEventListener('scroll', update)
    }
  }, [sections])

  const go = useCallback((id: string) => {
    refs.current[id]?.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' })
  }, [])

  return (
    <div ref={root} className="pb-20" style={{ background: LANDING.page }}>
      {hero(go)}
      <IconTabs tabs={tabs} current={current} onGo={go} />
      <div className="mx-auto max-w-6xl space-y-16">
        {sections.map((s) => (
          <section
            key={s.id}
            id={`${idPrefix}-${s.id}`}
            ref={(el) => {
              refs.current[s.id] = el
            }}
            aria-label={s.title}
            className="scroll-mt-24 space-y-4"
          >
            {renderSection(s.id, go)}
          </section>
        ))}
      </div>
    </div>
  )
}
