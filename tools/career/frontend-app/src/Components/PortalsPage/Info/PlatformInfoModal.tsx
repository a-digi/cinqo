import { useMemo } from 'react'
import { InfographicModal } from '../../../Shared/Infographic/InfographicModal'
import {
  CrawlTimeline,
  EnginesCompare,
  FirstStepsPath,
  HeroFlow,
  InstructionRecipe,
  LinkAnatomy,
  PlatformStack,
  TroubleTable,
} from './diagrams/diagrams'
import { accentOfChapter } from './infoTheme'
import { CHAPTERS, ENGINES_NOTE, HERO_LEAD, HERO_TITLE, MODAL_TITLE, type ChapterId } from './platformInfoData'
import { journeyProgress, journeyState, nextJourneyStep, type JourneyFacts } from './platformInfoRules'

// "How platforms work" (plan 262): the whole infographic in a full-page modal, built from the shared infographic parts (plan 266): the platform's chapters,
// its pictures and what the Platforms page already knows (`facts`) for the "Your first platform" path.
export function PlatformInfoModal({ open, onClose, facts }: { open: boolean; onClose: () => void; facts: JourneyFacts }) {
  const steps = useMemo(() => journeyState(facts), [facts])
  const progress = journeyProgress(facts)
  const nextId = nextJourneyStep(facts)
  return (
    <InfographicModal<ChapterId>
      open={open}
      onClose={onClose}
      title={MODAL_TITLE}
      chapters={CHAPTERS}
      accentOf={accentOfChapter}
      idPrefix="platform-info"
      hero={{ chapterId: 'overview', kicker: HERO_TITLE, lead: HERO_LEAD }}
      badge={(id) => (id === 'first-steps' ? `${String(progress.done)}/${String(progress.total)}` : null)}
      renderPicture={(id, go) => {
        switch (id) {
          case 'overview':
            return <HeroFlow onGo={go} />
          case 'platform':
            return <PlatformStack />
          case 'links':
            return <LinkAnatomy />
          case 'crawler':
            return <CrawlTimeline />
          case 'instructions':
            return (
              <div className="space-y-8">
                <InstructionRecipe />
                <EnginesCompare note={ENGINES_NOTE} />
              </div>
            )
          case 'first-steps':
            return <FirstStepsPath steps={steps} progress={progress} nextId={nextId} />
          case 'trouble':
            return <TroubleTable />
        }
      }}
    />
  )
}
