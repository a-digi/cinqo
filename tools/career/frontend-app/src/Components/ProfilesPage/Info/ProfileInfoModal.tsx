import { useMemo } from 'react'
import { Faq } from '../../../Shared/Infographic/landing/Faq'
import { BlueFrame } from '../../../Shared/Infographic/landing/BlueFrame'
import { LandingHero } from '../../../Shared/Infographic/landing/LandingHero'
import { LandingModal } from '../../../Shared/Infographic/landing/LandingModal'
import { PillButton } from '../../../Shared/Infographic/landing/PillButton'
import { ServiceCards } from '../../../Shared/Infographic/landing/ServiceCards'
import { Tile } from '../../../Shared/Infographic/landing/Tile'
import {
  CompletenessPicture,
  DataPicture,
  FirstStepsPath,
  HeroCluster,
  HeroFlow,
  JobsPicture,
  ProfileCardAnatomy,
  RolesPicture,
  RuleFrame,
  ScoreFrame,
} from './diagrams'
import { tabIconFor } from './tabIcons'
import { CHAPTERS, type Chapter, type ChapterId } from './profileInfoData'
import { FAQ_TITLE, LANDING_HERO, SECTIONS, SERVICE_CARDS, faqItems } from './profileLandingData'
import { captionRows, captionVariant, clickLine } from './profileLandingRules'
import { journeyProgress, journeyState, nextJourneyStep, type JourneyFacts } from './profileInfoRules'

const ID_PREFIX = 'profile-info'

// "How profiles work" in the landing look (plan 267): a navy hero, a strip of icon tabs, and the chapters as large pale-grey tiles whose pictures sit in
// blue-outlined frames, ending with a "Good to know" question section and two cards. The words, the checked captions and the pictures are those of plan
// 266; `facts` (from the completeness of the profiles the page loaded) feeds the "First steps" path. Platforms keeps the poster look.
export function ProfileInfoModal({ open, onClose, facts }: { open: boolean; onClose: () => void; facts: JourneyFacts }) {
  const steps = useMemo(() => journeyState(facts), [facts])
  const progress = journeyProgress(facts)
  const nextId = nextJourneyStep(facts)
  const chapterOf = (id: string): Chapter => CHAPTERS.find((c) => c.id === id) ?? CHAPTERS[0]

  const picture = (id: ChapterId, go: (id: string) => void): React.ReactNode => {
    switch (id) {
      case 'overview':
        return (
          <HeroFlow
            onGo={(c) => {
              go(c)
            }}
          />
        )
      case 'profile':
        return <ProfileCardAnatomy />
      case 'roles':
        return <RolesPicture />
      case 'data':
        return <DataPicture />
      case 'completeness':
        return <CompletenessPicture />
      case 'jobs':
        return <JobsPicture />
      case 'first-steps':
        return <FirstStepsPath steps={steps} progress={progress} nextId={nextId} />
      case 'careful':
        return null
    }
  }

  return (
    <LandingModal
      open={open}
      onClose={onClose}
      title={LANDING_HERO.title}
      idPrefix={ID_PREFIX}
      tabs={SECTIONS.map((s) => ({
        id: s.id,
        label: s.tab,
        icon: tabIconFor(s.icon),
        badge: s.id === 'first-steps' ? `${String(progress.done)}/${String(progress.total)}` : null,
      }))}
      sections={SECTIONS.map((s) => ({ id: s.id, title: chapterOf(s.id).title }))}
      hero={(go) => (
        <LandingHero
          titleId={`${ID_PREFIX}-hero-title`}
          title={LANDING_HERO.title}
          lead={LANDING_HERO.lead}
          ghost={LANDING_HERO.ghost}
          actions={
            <>
              <PillButton
                tone="light"
                arrow
                onClick={() => {
                  go(LANDING_HERO.start.section)
                }}
              >
                {LANDING_HERO.start.label}
              </PillButton>
              <button
                type="button"
                onClick={() => {
                  go(LANDING_HERO.picture.section)
                }}
                className="rounded-full border-2 border-white/60 px-5 py-2.5 text-sm font-bold text-white transition-colors hover:bg-white/10 motion-reduce:transition-none"
              >
                {LANDING_HERO.picture.label}
              </button>
            </>
          }
          picture={<HeroCluster />}
        />
      )}
      renderSection={(id, go) => {
        const layout = SECTIONS.find((s) => s.id === id)
        const chapter = chapterOf(id)
        if (!layout) return null
        if (layout.faq) {
          return (
            <div className="space-y-14 pt-6">
              <Faq idPrefix={`${ID_PREFIX}-faq`} title={FAQ_TITLE} items={faqItems()} startOpen="q1" />
              <ServiceCards
                cards={SERVICE_CARDS.map((c) => ({
                  id: c.id,
                  title: c.title,
                  text: c.text,
                  action: (
                    <PillButton href={c.href} arrow>
                      {c.button}
                    </PillButton>
                  ),
                }))}
              />
            </div>
          )
        }
        const index = SECTIONS.findIndex((s) => s.id === id)
        const line = clickLine(chapter.click)
        const rows = captionRows(chapter.captions.length)
        return (
          <>
            <Tile
              variant={layout.lead === 'wide' ? 'wide' : 'half'}
              titleId={`${ID_PREFIX}-${id}-title`}
              title={chapter.title}
              text={chapter.promise}
              actions={
                <>
                  {chapter.links.map((l) => (
                    <PillButton key={l.path} href={l.path} arrow>
                      {l.label}
                    </PillButton>
                  ))}
                  {line !== '' && <span className="text-sm font-medium text-gray-500">{line}</span>}
                </>
              }
              picture={<BlueFrame badge={String(index + 1)}>{picture(layout.id, go)}</BlueFrame>}
            />
            {chapter.captions.length > 0 && (
              <div className="grid gap-4 md:grid-cols-2" data-testid={`captions-${id}`} data-rows={rows.join(',')}>
                {chapter.captions.map((cap, i) => {
                  const variant = captionVariant(i, chapter.captions.length, layout.tallCaptions)
                  return (
                    <div key={cap.title} className={variant === 'wide' ? 'md:col-span-2' : ''}>
                      <Tile
                        variant={variant}
                        title={cap.title}
                        text={cap.text}
                        delay={i % 2}
                        picture={variant === 'tall' ? (i === 0 ? <ScoreFrame /> : <RuleFrame />) : undefined}
                      />
                    </div>
                  )
                })}
              </div>
            )}
          </>
        )
      }}
    />
  )
}
