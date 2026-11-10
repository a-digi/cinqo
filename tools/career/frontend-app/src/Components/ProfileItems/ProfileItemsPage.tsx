import { useEffect, useState, type ReactNode } from 'react'
import { PlusIcon } from '../../Shared/Icons/icons'
import { PageTips } from '../../Shared/PageTips/PageTips'
import type { PageTipsId } from '../../Shared/PageTips/pageTipData'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { buildProfileSearch } from '../ProfileLibrary/libraryRules'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { ProfileBar } from '../ProfileLibrary/ProfileBar'
import { useProfileChoice } from '../ProfileLibrary/useProfileChoice'
import { BackToProfile } from './BackToProfile'

// The list page of a profile's own items (languages, courses, hobbies; plan 231), the same layout as the Education page: the profile
// comes from the address (?profileId=), with a dropdown when there is more than one; Add and Edit open the item's entry page; Remove
// deletes at once. Each page says what its items look like (renderItem) and where its data comes from; nothing else differs.
export function ProfileItemsPage<T extends { id: string }>({
  path,
  entryPath,
  icon,
  title,
  description,
  tipsPage,
  addLabel,
  hint,
  emptyText,
  countText,
  fetchItems,
  removeItem,
  labelOf,
  renderItem,
}: {
  path: string
  entryPath: string
  icon: ReactNode
  title: string
  description: string
  tipsPage: PageTipsId
  addLabel: string
  hint: string
  emptyText: string
  countText: (count: number) => string
  fetchItems: (profileId: string) => Promise<T[]>
  removeItem: (id: string) => Promise<void>
  // The text the item's round badge takes its letter and colour from.
  labelOf: (item: T) => string
  renderItem: (item: T) => ReactNode
}) {
  const { profiles, profileId, select, error: loadError, loaded } = useProfileChoice(path)
  const [items, setItems] = useState<T[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (profileId === '') return
    setError('')
    fetchItems(profileId)
      .then(setItems)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // fetchItems is a module function of the page: it never changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profileId])

  function openEntry(id?: string) {
    const idPart = id ? `&id=${encodeURIComponent(id)}` : ''
    window.__cinqoToolBridge.navigate(`${entryPath}${buildProfileSearch(profileId)}${idPart}`)
  }

  function handleRemove(id: string) {
    setError('')
    removeItem(id)
      .then(() => {
        setItems((prev) => prev.filter((e) => e.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const shownError = loadError !== '' ? loadError : error

  return (
    <div className="career-v-zoom career-a-float career-t-type max-w-5xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} profiles={profiles} />}
      <PageHeaderCard
        icon={icon}
        title={title}
        help={path}
        description={description}
        pill={loaded && shownError === '' && profileId !== '' ? { text: countText(items.length) } : undefined}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {shownError !== '' && <p className="mb-3 text-sm text-red-700">{shownError}</p>}
          {loaded && loadError === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There is no profile yet. Create one on the Profiles page first.
            </p>
          )}
          {profileId !== '' && (
            <>
              <ProfileBar
                profiles={profiles}
                personas={[]}
                profileId={profileId}
                onSelect={select}
                personaHref={() => path}
                personaLabel=""
              />
              <div className="mb-3 flex items-center justify-start gap-3">
                <button
                  type="button"
                  onClick={() => {
                    openEntry()
                  }}
                  className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
                >
                  <PlusIcon />
                  {addLabel}
                </button>
                <p className="min-w-0 flex-1 text-xs text-gray-500">{hint}</p>
              </div>
              {items.length === 0 && shownError === '' && (
                <div
                  className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
                  style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
                >
                  {emptyText}
                </div>
              )}
              <div className="career-stagger space-y-3">
                {items.map((item) => (
                  <div
                    key={item.id}
                    className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                    style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                  >
                    <div className="flex items-start gap-3">
                      <span
                        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                        style={{ background: accentFor(labelOf(item)).to }}
                        aria-hidden="true"
                      >
                        {initialOf(labelOf(item))}
                      </span>
                      <div className="min-w-0 flex-1">{renderItem(item)}</div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            openEntry(item.id)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleRemove(item.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page={tipsPage} orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
