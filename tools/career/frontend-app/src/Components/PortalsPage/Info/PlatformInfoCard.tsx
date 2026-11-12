import { InfoCard } from '../../../Shared/Infographic/InfoCard'
import { GlobeIcon } from '../../../Shared/Icons/icons'
import { ACCENTS } from '../../JobsPage/jobsTheme'
import { CARD_BUTTON, CARD_TEXT, CARD_TITLE, HERO_NODES } from './platformInfoData'

// The small, still area in the left column of the Platforms page (plan 262): a title, one line, the first three parts of the picture and a button that
// opens the whole infographic. Not a slider: nothing moves by itself.
export function PlatformInfoCard({ onOpen }: { onOpen: () => void }) {
  return (
    <InfoCard
      idPrefix="platform"
      accent={ACCENTS[0]}
      icon={<GlobeIcon className="h-4 w-4" />}
      title={CARD_TITLE}
      text={CARD_TEXT}
      parts={HERO_NODES.slice(0, 3).map((p) => p.label)}
      buttonLabel={CARD_BUTTON}
      onOpen={onOpen}
    />
  )
}
