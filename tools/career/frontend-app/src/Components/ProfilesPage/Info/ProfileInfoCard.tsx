import { UserIcon } from '../../../Shared/Icons/icons'
import { InfoCard } from '../../../Shared/Infographic/InfoCard'
import { ACCENTS } from '../../JobsPage/jobsTheme'
import { CARD_BUTTON, CARD_TEXT, CARD_TITLE, HERO_NODES } from './profileInfoData'

// The small, still area in the left column of the Profiles page (plan 266): a title, one line, the first three parts of the picture and a button that
// opens the whole infographic. Not a slider: nothing moves by itself.
export function ProfileInfoCard({ onOpen }: { onOpen: () => void }) {
  return (
    <InfoCard
      idPrefix="profile"
      accent={ACCENTS[0]}
      icon={<UserIcon className="h-4 w-4" />}
      title={CARD_TITLE}
      text={CARD_TEXT}
      parts={HERO_NODES.slice(0, 3).map((p) => p.label)}
      buttonLabel={CARD_BUTTON}
      onOpen={onOpen}
    />
  )
}
