import { InfoTeaser } from '../../../Shared/Infographic/InfoTeaser'
import { PlusIcon } from '../../../Shared/Icons/icons'
import { ACCENTS } from '../../JobsPage/jobsTheme'
import { INFO_BUTTON, TEASER_LEAD, TEASER_STEPS, TEASER_TITLE } from './platformInfoData'

// What a page with no platform shows instead of an empty list (plan 262): the first three steps in one small picture and a button that opens the whole
// infographic. It is gone as soon as there is a platform (the page decides: infoEntryFor).
export function PlatformInfoTeaser({ onOpen, onNew }: { onOpen: () => void; onNew: () => void }) {
  return (
    <InfoTeaser
      idPrefix="platform"
      accent={ACCENTS[0]}
      title={TEASER_TITLE}
      lead={TEASER_LEAD}
      steps={TEASER_STEPS}
      openLabel={INFO_BUTTON}
      onOpen={onOpen}
      actions={
        <button
          type="button"
          onClick={onNew}
          className="flex items-center gap-1.5 rounded-full border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50"
        >
          <PlusIcon />
          New platform
        </button>
      }
    />
  )
}
