import { mediaDownloadUrl, type Profile } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { profileLabel } from '../ProfileSwitcher/profileLabel'

// The profiles as a list, each a link to a page of the caller's choosing: the one shared way to pick a profile
// without a dropdown. The page supplies where an item leads (hrefFor) and what is shown under the name
// (renderExtra: a count, a few names). The item shows the profile's photo, or its initial.
export function ProfileList({
  profiles,
  hrefFor,
  renderExtra,
}: {
  profiles: Profile[]
  hrefFor: (profile: Profile) => string
  renderExtra?: (profile: Profile) => React.ReactNode
}) {
  const sorted = [...profiles].sort((a, b) => profileLabel(a).localeCompare(profileLabel(b), undefined, { sensitivity: 'base' }))
  return (
    <ul className="career-stagger space-y-2.5">
      {sorted.map((p) => {
        const name = profileLabel(p)
        return (
          <li key={p.id}>
            <AppLink
              href={hrefFor(p)}
              className="flex items-start gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
              style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
            >
              {p.imageMediaFileId ? (
                <img src={mediaDownloadUrl(p.imageMediaFileId)} alt="" className="h-12 w-12 shrink-0 rounded-full object-cover shadow-sm" />
              ) : (
                <span
                  className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-bold text-white shadow-sm"
                  style={{ background: accentFor(name).to }}
                  aria-hidden="true"
                >
                  {initialOf(name)}
                </span>
              )}
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold text-gray-900">{name}</span>
                {renderExtra?.(p)}
              </span>
            </AppLink>
          </li>
        )
      })}
    </ul>
  )
}
