import { useMemo } from 'react'
import { InfographicModal } from '../../../Shared/Infographic/InfographicModal'
import {
  CarefulPicture,
  CompletenessPicture,
  DataPicture,
  FirstStepsPath,
  HeroFlow,
  JobsPicture,
  ProfileCardAnatomy,
  RolesPicture,
} from './diagrams'
import { accentOfChapter } from './infoTheme'
import { CHAPTERS, HERO_LEAD, HERO_TITLE, MODAL_TITLE, type ChapterId } from './profileInfoData'
import { journeyProgress, journeyState, nextJourneyStep, type JourneyFacts } from './profileInfoRules'

// "How profiles work" (plan 266): the whole infographic in a full-page modal, built from the shared infographic parts: the profiles' chapters, their
// pictures and what the Profiles page already knows (`facts`, from the completeness of the profiles it loaded) for the "Your first profile" path.
export function ProfileInfoModal({ open, onClose, facts }: { open: boolean; onClose: () => void; facts: JourneyFacts }) {
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
      idPrefix="profile-info"
      hero={{ chapterId: 'overview', kicker: HERO_TITLE, lead: HERO_LEAD }}
      badge={(id) => (id === 'first-steps' ? `${String(progress.done)}/${String(progress.total)}` : null)}
      renderPicture={(id, go) => {
        switch (id) {
          case 'overview':
            return <HeroFlow onGo={go} />
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
            return <CarefulPicture />
        }
      }}
    />
  )
}
