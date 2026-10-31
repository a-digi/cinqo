import { mediaDownloadUrl, type Photo } from '../../api'
import { TrashIcon } from '../../Shared/Icons/icons'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { usedByLabel, usedByTitle } from './photoRules'

// The photos that already exist (step 110): pick one to use it for the chosen profile, or
// delete one for good. A photo can be used by many profiles. A uniquely-named grid class, as
// the other Career pages do: the globally-injected Tailwind stylesheet can collapse `.grid-cols-*`.
export function PhotoLibrary({
  photos,
  currentMediaFileId,
  cacheBuster,
  busy,
  onPick,
  onDelete,
}: {
  photos: Photo[]
  currentMediaFileId: string
  cacheBuster: number
  busy: boolean
  onPick: (photo: Photo) => void
  onDelete: (photo: Photo) => void
}) {
  if (photos.length === 0) return null
  return (
    <section
      aria-label="Your photos"
      className="career-enter career-delay-2 rounded-3xl border bg-white p-5 shadow-sm"
      style={{ borderColor: PAGE_ACCENT.soft }}
    >
      <style>{`
        .photo-library-grid { display: grid; gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr)); }
      `}</style>
      <h2 className="text-sm font-semibold text-gray-700">Your photos</h2>
      <p className="mb-3 mt-0.5 text-xs text-gray-500">Pick one to use it for this profile. Many profiles can use the same photo.</p>
      <ul className="photo-library-grid">
        {photos.map((photo) => {
          const current = photo.mediaFileId === currentMediaFileId
          return (
            <li key={photo.mediaFileId} className="group relative">
              <button
                type="button"
                disabled={busy || current}
                onClick={() => {
                  onPick(photo)
                }}
                aria-pressed={current}
                aria-label={`${current ? 'Current photo of this profile' : 'Use for this profile'}. ${usedByLabel(photo.usedBy)}`}
                title={usedByTitle(photo.usedBy)}
                className="block w-full text-left disabled:cursor-default"
              >
                <span
                  className={`relative block aspect-square overflow-hidden rounded-2xl border-2 bg-gray-100 transition-shadow ${
                    current ? 'shadow-md' : 'border-transparent hover:shadow-md'
                  }`}
                  style={current ? { borderColor: PAGE_ACCENT.to } : undefined}
                >
                  <img src={`${mediaDownloadUrl(photo.mediaFileId)}?v=${cacheBuster}`} alt="" className="h-full w-full object-cover" />
                  {current && (
                    <span
                      className="absolute bottom-1 left-1 rounded-full px-2 py-0.5 text-[10px] font-semibold text-white"
                      style={{ background: PAGE_ACCENT.to }}
                    >
                      This profile
                    </span>
                  )}
                </span>
                <span className="mt-1 block truncate text-xs text-gray-500">{usedByLabel(photo.usedBy)}</span>
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  onDelete(photo)
                }}
                aria-label="Delete this photo for good"
                title="Delete this photo for good"
                className="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-white/90 text-red-700 opacity-0 shadow-sm transition-opacity hover:bg-red-50 focus:opacity-100 group-hover:opacity-100 disabled:opacity-0 [@media(hover:none)]:opacity-100"
              >
                <TrashIcon />
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
