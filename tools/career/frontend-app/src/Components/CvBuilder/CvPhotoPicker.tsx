import { mediaDownloadUrl } from '../../api'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import type { CvPhoto } from './cvPhotoRules'

// Which of the profile's photos this CV uses (step 111): shown under the "include the photo"
// checkbox when the profile has more than one. One choice, the default preselected.
export function CvPhotoPicker({
  photos,
  selected,
  onSelect,
}: {
  photos: CvPhoto[]
  selected: string
  onSelect: (mediaFileId: string) => void
}) {
  return (
    <div role="radiogroup" aria-label="Photo for this CV" className="flex flex-wrap gap-3 pl-6">
      {photos.map((photo) => {
        const checked = photo.mediaFileId === selected
        return (
          <button
            key={photo.mediaFileId}
            type="button"
            role="radio"
            aria-checked={checked}
            aria-label={photo.isDefault ? 'Default photo' : 'Photo'}
            onClick={() => {
              onSelect(photo.mediaFileId)
            }}
            className="flex w-20 flex-col items-center gap-1 text-xs text-gray-600"
          >
            <span
              className={`block h-20 w-20 overflow-hidden rounded-2xl border-2 bg-gray-100 transition-shadow ${checked ? 'shadow-md' : 'border-transparent hover:shadow-md'}`}
              style={checked ? { borderColor: PAGE_ACCENT.to } : undefined}
            >
              <img src={mediaDownloadUrl(photo.mediaFileId)} alt="" className="h-full w-full object-cover" />
            </span>
            <span className={checked ? 'font-semibold text-gray-900' : ''}>{photo.isDefault ? 'Default' : checked ? 'Selected' : ' '}</span>
          </button>
        )
      })}
    </div>
  )
}
