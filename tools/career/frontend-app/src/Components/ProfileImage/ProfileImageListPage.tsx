import { useEffect, useState } from 'react'
import { fetchProfilePhotos, fetchProfiles, type Profile, type ProfilePhoto } from '../../api'
import { CameraIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PROFILES_PATH, photosPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { ProfileList } from '../ProfilesPage/ProfileList'

// The Photo page: the profiles as a list, each with how many photos it has; a click opens that profile's own
// photo page (/tools/career/profile-image/:profileId), where photos are added, chosen as the default, removed
// and deleted. Nothing is edited here, and there is no dropdown. See
// plan/ai/tools/career/step-125-photo-per-profile-pages.md.
export function ProfileImageListPage() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Each profile's photos by profile id; a profile whose photos could not be read is missing.
  const [photos, setPhotos] = useState<Record<string, ProfilePhoto[] | undefined>>({})
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    fetchProfiles()
      .then((list) => {
        setProfiles(list)
        void Promise.all(list.map((p) => fetchProfilePhotos(p.id).catch(() => null))).then((all) => {
          const next: Record<string, ProfilePhoto[] | undefined> = {}
          list.forEach((p, i) => {
            const result = all[i]
            if (result) next[p.id] = result
          })
          setPhotos(next)
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  return (
    <div className="career-v-stretch career-a-float career-t-wave max-w-5xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<CameraIcon />}
        title="Photo"
        description="Pick a profile to see and change its photos, used by the CV templates. A profile can have several, one of them the default; one photo can be shared by many profiles."
        pill={loaded && error === '' ? { text: plural(profiles.length, 'profile', 'profiles') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There are no profiles yet.{' '}
              <a
                href={PROFILES_PATH}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(PROFILES_PATH)
                }}
                className="font-medium text-blue-700 hover:underline"
              >
                Create one on the Profiles page
              </a>
              , then add its photos here.
            </p>
          )}
          {profiles.length > 0 && (
            <ProfileList
              profiles={profiles}
              hrefFor={(p) => photosPathFor(p.id)}
              renderExtra={(p) => {
                const list = photos[p.id]
                if (!list) return null
                return (
                  <span className="mt-1.5 block text-xs text-gray-500">
                    <span className="font-medium text-gray-700">
                      {list.length === 0 ? 'No photo' : plural(list.length, 'photo', 'photos')}
                    </span>
                  </span>
                )
              }}
            />
          )}
        </div>
        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="photo" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
