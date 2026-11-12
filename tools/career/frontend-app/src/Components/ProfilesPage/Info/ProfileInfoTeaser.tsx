import { AppLink } from '../../../Shared/AppLink/AppLink'
import { PlusIcon, UploadIcon } from '../../../Shared/Icons/icons'
import { InfoTeaser } from '../../../Shared/Infographic/InfoTeaser'
import { GUIDE_PATH, importCvPathFor } from '../../../paths'
import { ACCENTS } from '../../JobsPage/jobsTheme'
import { INFO_BUTTON, TEASER_LEAD, TEASER_STEPS, TEASER_TITLE } from './profileInfoData'

// What a page with no profile shows instead of the grey sentence (plan 266): the first three steps in one small picture, a button that opens the whole
// infographic, and the page's own ways in: New profile, Import CV and the getting-started guide. It is gone as soon as there is a profile (the page
// decides: infoEntryFor).
export function ProfileInfoTeaser({ onOpen, onNew }: { onOpen: () => void; onNew: () => void }) {
  return (
    <InfoTeaser
      idPrefix="profile"
      accent={ACCENTS[0]}
      title={TEASER_TITLE}
      lead={TEASER_LEAD}
      steps={TEASER_STEPS}
      openLabel={INFO_BUTTON}
      onOpen={onOpen}
      actions={
        <>
          <button
            type="button"
            onClick={onNew}
            className="flex items-center gap-1.5 rounded-full border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50"
          >
            <PlusIcon />
            New profile
          </button>
          <AppLink
            href={importCvPathFor()}
            className="flex items-center gap-1.5 rounded-full border border-blue-200 bg-white px-4 py-2 text-sm font-medium text-blue-700 hover:bg-blue-50"
          >
            <UploadIcon className="h-4 w-4" />
            Import CV
          </AppLink>
          <AppLink href={GUIDE_PATH} className="text-sm font-medium text-blue-700 hover:underline">
            Getting started →
          </AppLink>
        </>
      }
    />
  )
}
