import { useMemo } from 'react'
import { PLATFORMS_PATH } from '../../../paths'
import { AppLink } from '../../../Shared/AppLink/AppLink'
import { Band } from '../../../Shared/Infographic/showcase/Band'
import { BigNumber } from '../../../Shared/Infographic/showcase/BigNumber'
import { ClosingLine } from '../../../Shared/Infographic/showcase/ClosingLine'
import { Footnotes } from '../../../Shared/Infographic/showcase/Footnotes'
import { IconRow } from '../../../Shared/Infographic/showcase/IconRow'
import { PinnedStage } from '../../../Shared/Infographic/showcase/PinnedStage'
import { Segmented } from '../../../Shared/Infographic/showcase/Segmented'
import { ShowcaseHero } from '../../../Shared/Infographic/showcase/ShowcaseHero'
import { ShowcaseModal } from '../../../Shared/Infographic/showcase/ShowcaseModal'
import { ShowcasePill } from '../../../Shared/Infographic/showcase/ShowcasePill'
import { toneColors, type ShowcaseTone } from '../../../Shared/Infographic/showcase/showcaseTokens'
import { WhyGrid } from '../../../Shared/Infographic/showcase/WhyGrid'
import {
  APPLICATION_ITEMS,
  BAR_NAME,
  CLEANUP_LINK,
  CLOSING_LINE,
  DETAIL_CAPTIONS,
  FIND_ITEMS,
  FOOTNOTES,
  GOOD_CARDS,
  HERO_BUTTON,
  HERO_LEAD,
  HERO_LINK,
  HERO_TITLE,
  HOLDS,
  MATCH_CAPTIONS,
  MATCH_NUMBER,
  MODAL_TITLE,
  NOT_JUDGED_NOTE,
  SECTIONS,
  TEASER_BUTTONS,
  TWO_WAYS,
  WAY_IN_STEPS,
  type SectionId,
} from './jobInfoData'
import { journeyProgress, journeyState, nextJourneyStep, type JobFacts } from './jobInfoRules'
import { jobIconFor } from './jobIcons'
import { BoardPicture, DetailsPicture, FindPicture, HeroCards, JobFirstSteps, MatchPicture, WayPanel, WayPicture } from './pictures'

const ID_PREFIX = 'job-info'

// A row of short captions under a picture (title and one line each), in the colours of the band.
function Captions({ items, tone }: { items: { title: string; text: string }[]; tone: ShowcaseTone }) {
  const c = toneColors(tone)
  return (
    <ul
      className="mx-auto mt-14 grid max-w-6xl gap-x-10 gap-y-8 text-left sm:grid-cols-2 lg:grid-cols-[repeat(auto-fit,minmax(14rem,1fr))]"
      data-testid="captions"
    >
      {items.map((it) => (
        <li key={it.title}>
          <h4 className="text-xl font-semibold" style={{ color: c.color }}>
            {it.title}
          </h4>
          <p className="mt-2 text-base leading-relaxed" style={{ color: c.muted }}>
            {it.text}
          </p>
        </li>
      ))}
    </ul>
  )
}

// "How jobs work" in the showcase look (plan 269): a black hero, the bar with the chapters and the blue "Add job", nine bands (the way in as a pinned stage, two ways in as tabs, ...), the footnotes
// and the closing line. The words are jobInfoData's, the pictures pictures.tsx's; `facts` (from what the Jobs page loaded) feeds the first steps. "Add job" closes this page and opens the Jobs
// page's own Add job dialog (`onAddJob`).
export function JobInfoModal({
  open,
  onClose,
  facts,
  onAddJob,
}: {
  open: boolean
  onClose: () => void
  facts: JobFacts
  onAddJob: () => void
}) {
  const steps = useMemo(() => journeyState(facts), [facts])
  const progress = journeyProgress(facts)
  const nextId = nextJourneyStep(facts)
  const section = (id: SectionId) => SECTIONS.find((s) => s.id === id) ?? SECTIONS[0]

  const content = (id: SectionId): React.ReactNode => {
    const tone = section(id).tone
    switch (id) {
      case 'way-in':
        return (
          <PinnedStage
            idPrefix={`${ID_PREFIX}-way`}
            steps={WAY_IN_STEPS}
            tone={tone}
            picture={(step, progress) => <WayPicture step={step} progress={progress} />}
          />
        )
      case 'two-ways':
        return (
          <Segmented
            idPrefix={`${ID_PREFIX}-ways`}
            tone={tone}
            label="How a job gets in"
            tabs={TWO_WAYS.map((w) => ({
              id: w.id,
              label: w.label,
              panel: (
                <div>
                  <WayPanel id={w.id} />
                  <p className="mx-auto mt-10 max-w-2xl text-2xl font-semibold text-white">{w.tagline}</p>
                  <ul className="mx-auto mt-6 max-w-2xl space-y-3 text-left text-lg leading-snug text-white/80">
                    {w.points.map((p) => (
                      <li key={p} className="flex gap-3">
                        <span aria-hidden="true" className="text-[#2997ff]">
                          ✓
                        </span>
                        <span>{p}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              ),
            }))}
          />
        )
      case 'holds':
        return (
          <IconRow
            tone={tone}
            items={HOLDS.map((h) => ({ id: h.id, icon: jobIconFor(h.icon, 'h-10 w-10'), title: h.title, text: h.text }))}
          />
        )
      case 'match':
        return (
          <div>
            <BigNumber value={MATCH_NUMBER.value} label={MATCH_NUMBER.label} tone={tone} />
            <div className="mt-16">
              <MatchPicture />
            </div>
            <Captions items={MATCH_CAPTIONS} tone={tone} />
            <p className="mx-auto mt-10 max-w-3xl text-sm" style={{ color: toneColors(tone).muted }}>
              {NOT_JUDGED_NOTE}
            </p>
          </div>
        )
      case 'details':
        return (
          <div>
            <DetailsPicture />
            <Captions items={DETAIL_CAPTIONS} tone={tone} />
          </div>
        )
      case 'find':
        return (
          <div>
            <FindPicture />
            <Captions items={FIND_ITEMS} tone={tone} />
          </div>
        )
      case 'application':
        return (
          <div>
            <BoardPicture />
            <div className="mt-16">
              <IconRow
                tone={tone}
                items={APPLICATION_ITEMS.map((a) => ({ id: a.id, icon: jobIconFor(a.icon, 'h-10 w-10'), title: a.title, text: a.text }))}
              />
            </div>
          </div>
        )
      case 'first-steps':
        return (
          <div>
            <JobFirstSteps steps={steps} progress={progress} nextId={nextId} />
            <p className="mt-10">
              <ShowcasePill href={PLATFORMS_PATH} arrow>
                {nextId === 'platform' || nextId === null ? TEASER_BUTTONS.platform : TEASER_BUTTONS.platforms}
              </ShowcasePill>
            </p>
          </div>
        )
      case 'good':
        return (
          <div>
            <WhyGrid
              cards={GOOD_CARDS.map((g) => ({ id: g.id, icon: jobIconFor(g.icon, 'h-8 w-8'), title: g.title, text: g.text, note: g.note }))}
            />
            <p className="mt-8 text-center text-base">
              <AppLink href={CLEANUP_LINK.path} className="text-[#0071e3] hover:underline">
                Allow a deleted job again under Settings → {CLEANUP_LINK.label}
              </AppLink>
            </p>
            <div className="mt-12">
              <Footnotes notes={FOOTNOTES} />
            </div>
          </div>
        )
    }
  }

  return (
    <ShowcaseModal
      open={open}
      onClose={onClose}
      title={MODAL_TITLE}
      idPrefix={ID_PREFIX}
      barName={BAR_NAME}
      tabs={SECTIONS.map((s) => ({ id: s.id, label: s.nav }))}
      cta={{ label: TEASER_BUTTONS.add, onClick: onAddJob }}
      sections={SECTIONS.map((s) => ({ id: s.id, title: s.title, tone: s.tone }))}
      hero={(go) => (
        <ShowcaseHero
          titleId={`${ID_PREFIX}-hero-title`}
          title={HERO_TITLE}
          lead={HERO_LEAD}
          actions={
            <>
              <ShowcasePill
                onClick={() => {
                  go('way-in')
                }}
                arrow
              >
                {HERO_BUTTON}
              </ShowcasePill>
              <ShowcasePill tone="outline-dark" href={HERO_LINK.path}>
                {HERO_LINK.label}
              </ShowcasePill>
            </>
          }
          picture={<HeroCards />}
        />
      )}
      renderSection={(id) => {
        const s = section(id as SectionId)
        return (
          <Band tone={s.tone} eyebrow={s.eyebrow} title={s.title} titleId={`${ID_PREFIX}-${s.id}-title`} lead={s.lead}>
            {content(s.id)}
          </Band>
        )
      }}
      closing={() => (
        <ClosingLine
          line={CLOSING_LINE}
          actions={
            <>
              <ShowcasePill href={PLATFORMS_PATH}>{TEASER_BUTTONS.platform}</ShowcasePill>
              <ShowcasePill tone="outline-dark" onClick={onAddJob}>
                {TEASER_BUTTONS.add}
              </ShowcasePill>
            </>
          }
        />
      )}
    />
  )
}
