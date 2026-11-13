import { useCallback, useEffect, useRef, useState } from 'react'
import { AppLink } from '../../AppLink/AppLink'
import { Modal } from '../../Modal/Modal'
import { chapterInView } from '../infographicRules'
import { BAR_LINE, SETTLE_PX, prefersReducedMotion, scrollParentOf, stripScrollFor } from '../scrollRules'
import { SHOWCASE, toneColors, type ShowcaseTone } from './showcaseTokens'

export interface ShowcaseSection {
  id: string
  // The full heading of the section: the region is named by it.
  title: string
  tone: ShowcaseTone
}

export interface ShowcaseTab {
  id: string
  label: string
}

export interface ShowcaseCta {
  label: string
  onClick?: () => void
  // A real page of the tool instead of an action.
  href?: string
}

export interface ShowcaseProps {
  open: boolean
  onClose: () => void
  // The title of the full-page modal (its own header with Close stays above the hero).
  title: string
  // The start of every id ("job-info": "job-info-matches").
  idPrefix: string
  // The name on the left of the local bar, its anchors, and the one blue button at its right.
  barName: string
  tabs: ShowcaseTab[]
  cta: ShowcaseCta
  hero: (go: (id: string) => void) => React.ReactNode
  sections: ShowcaseSection[]
  renderSection: (id: string, go: (id: string) => void) => React.ReactNode
  // After the sections: the footnotes and the closing line.
  closing?: (go: (id: string) => void) => React.ReactNode
}

// A "How it works" page in the showcase look (plan 269): a black hero, a thin translucent bar that sticks to the top (the page's name, the chapters as quiet links with the one in view
// dark, and a blue button), then the sections one under the other as full-width bands, black and light in turn. The content is given; this lays it out, follows the scroll and scrolls to
// a chapter on a click.
export function ShowcaseModal(props: ShowcaseProps) {
  return (
    <Modal open={props.open} title={props.title} onClose={props.onClose} size="full">
      <Page {...props} />
    </Modal>
  )
}

function Page({ idPrefix, barName, tabs, cta, hero, sections, renderSection, closing }: ShowcaseProps) {
  const root = useRef<HTMLDivElement>(null)
  const refs = useRef<Record<string, HTMLElement | null>>({})
  const [current, setCurrent] = useState(0)
  const barList = useRef<HTMLUListElement>(null)
  const barItems = useRef<(HTMLLIElement | null)[]>([])

  useEffect(() => {
    const scroller = scrollParentOf(root.current)
    if (!scroller) return
    const update = () => {
      const top = scroller.getBoundingClientRect().top
      const tops = sections.map((s) => (refs.current[s.id]?.getBoundingClientRect().top ?? Number.POSITIVE_INFINITY) - top)
      setCurrent(chapterInView(tops, BAR_LINE + SETTLE_PX))
    }
    update()
    scroller.addEventListener('scroll', update, { passive: true })
    return () => {
      scroller.removeEventListener('scroll', update)
    }
  }, [sections])

  // On a narrow window the bar scrolls sideways: bring the chapter in view into view in it (sideways only, the page is not moved).
  const currentId = sections[current]?.id
  useEffect(() => {
    const ul = barList.current
    const at = tabs.findIndex((t) => t.id === currentId)
    const li = at >= 0 ? barItems.current[at] : null
    if (!ul || !li) return
    const left = stripScrollFor(ul.scrollLeft, ul.clientWidth, ul.scrollWidth, li.offsetLeft, li.offsetWidth)
    if (left !== ul.scrollLeft) ul.scrollTo({ left, behavior: prefersReducedMotion() ? 'auto' : 'smooth' })
  }, [currentId, tabs])

  const go = useCallback((id: string) => {
    refs.current[id]?.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' })
  }, [])

  const sectionIndex = (tabId: string): number => sections.findIndex((s) => s.id === tabId)

  return (
    <div ref={root} className="pb-0" style={{ background: SHOWCASE.black, fontFamily: SHOWCASE.font }}>
      {hero(go)}
      <nav
        aria-label="Chapters"
        data-testid="local-bar"
        className="sticky -top-5 z-20 -mx-6 border-b px-6 backdrop-blur-xl"
        style={{ background: 'rgba(22,22,23,0.8)', borderColor: 'rgba(255,255,255,0.12)' }}
      >
        <div className="mx-auto flex h-12 max-w-6xl items-center gap-6">
          <span className="shrink-0 text-base font-semibold text-white" data-testid="local-bar-name">
            {barName}
          </span>
          <ul ref={barList} className="relative flex min-w-0 flex-1 items-center gap-5 overflow-x-auto text-xs md:justify-end md:gap-6">
            {tabs.map((t, i) => {
              const on = sectionIndex(t.id) === current
              return (
                <li
                  key={t.id}
                  className="shrink-0"
                  ref={(el) => {
                    barItems.current[i] = el
                  }}
                >
                  <button
                    type="button"
                    onClick={() => {
                      go(t.id)
                    }}
                    aria-current={on ? 'true' : undefined}
                    className="py-1 transition-colors motion-reduce:transition-none"
                    style={{ color: on ? '#ffffff' : SHOWCASE.mutedOnDark }}
                  >
                    {t.label}
                  </button>
                </li>
              )
            })}
          </ul>
          <BarButton cta={cta} />
        </div>
      </nav>
      {sections.map((s) => {
        const c = toneColors(s.tone)
        return (
          <section
            key={s.id}
            id={`${idPrefix}-${s.id}`}
            ref={(el) => {
              refs.current[s.id] = el
            }}
            aria-label={s.title}
            data-tone={s.tone}
            className="-mx-6 scroll-mt-12 px-6 py-24 md:py-36"
            style={{ background: c.background, color: c.color }}
          >
            {renderSection(s.id, go)}
          </section>
        )
      })}
      {closing?.(go)}
    </div>
  )
}

function BarButton({ cta }: { cta: ShowcaseCta }) {
  const className =
    'shrink-0 rounded-full px-4 py-1 text-xs font-medium text-white transition-colors hover:brightness-110 motion-reduce:transition-none'
  const style = { background: SHOWCASE.blue }
  if (cta.href !== undefined) {
    return (
      <AppLink href={cta.href} className={className} style={style}>
        {cta.label}
      </AppLink>
    )
  }
  return (
    <button type="button" onClick={cta.onClick} className={className} style={style}>
      {cta.label}
    </button>
  )
}
