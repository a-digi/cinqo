import { BriefcaseIcon } from '../../../Shared/Icons/icons'
import { InfoCard } from '../../../Shared/Infographic/InfoCard'
import { ACCENTS } from '../jobsTheme'
import { CARD_BUTTON, CARD_TEXT, CARD_TITLE, WAY_IN_STEPS } from './jobInfoData'

// The small, still card at the top of the Jobs page's side column (plan 269): a title, one line, the first three steps of the way in and a button that opens the whole infographic.
export function JobInfoCard({ onOpen }: { onOpen: () => void }) {
  return (
    <InfoCard
      idPrefix="job"
      accent={ACCENTS[0]}
      icon={<BriefcaseIcon className="h-4 w-4" />}
      title={CARD_TITLE}
      text={CARD_TEXT}
      parts={WAY_IN_STEPS.slice(0, 3).map((s) => s.title)}
      buttonLabel={CARD_BUTTON}
      onOpen={onOpen}
    />
  )
}
