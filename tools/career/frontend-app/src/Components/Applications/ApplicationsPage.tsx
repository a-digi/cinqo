import { useEffect, useState } from 'react'
import { fetchJobApplications, fetchProfiles, type JobApplication, type Profile } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { BriefcaseIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { jobBoardPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { columnAccent } from '../JobBoard/jobBoardRules'
import { jobDetailsPathFor } from '../JobDetails/jobProfileScope'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { SORTS, activityText, protocolText, sortApplications, waitingCount, type ApplicationSort } from './applicationsRules'

interface ProfileApplications {
  profile: Profile
  applications: JobApplication[]
}

// The overview of the applications (/tools/career/applications, plan 176 step 5): per profile, the jobs it applied to (Applied on its
// board) or has a protocol for, with the status, what the protocol says, when something last happened and whether the other side
// owes a reply. Each row opens the job for that profile, where the protocol is. Read only: everything is changed on the job page.
export function ApplicationsPage() {
  const [groups, setGroups] = useState<ProfileApplications[] | null>(null)
  const [error, setError] = useState('')
  const [sort, setSort] = useState<ApplicationSort>('recent')

  useEffect(() => {
    fetchProfiles()
      .then((profiles) => Promise.all(profiles.map(async (profile) => ({ profile, applications: await fetchJobApplications(profile.id) }))))
      .then(setGroups)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  const now = new Date()
  const shown = (groups ?? []).filter((g) => g.applications.length > 0)
  const total = shown.reduce((sum, g) => sum + g.applications.length, 0)
  const waiting = shown.reduce((sum, g) => sum + waitingCount(g.applications), 0)

  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-full p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<BriefcaseIcon />}
        title="Applications"
        description="The jobs each profile applied to or keeps a protocol for, and who owes the next message."
        pill={{
          text:
            waiting > 0
              ? `${plural(total, 'application', 'applications')} · ${String(waiting)} waiting`
              : plural(total, 'application', 'applications'),
        }}
      />
      {error !== '' && (
        <p role="alert" className="mb-3 text-sm text-red-700">
          {error}
        </p>
      )}
      {groups === null && error === '' && <p className="text-sm text-gray-500">Loading…</p>}
      {groups !== null && shown.length === 0 && (
        <p className="rounded-3xl border border-gray-100 bg-gray-50 p-6 text-center text-sm text-gray-500">
          No applications yet. A job shows here once it is in the Applied status on a job board, or has an entry in its protocol.
        </p>
      )}
      {shown.length > 0 && (
        <div className="mb-3 flex flex-wrap items-center gap-2 text-xs text-gray-500">
          <label htmlFor="applications-sort">Order by</label>
          <select
            id="applications-sort"
            value={sort}
            onChange={(e) => {
              setSort(e.target.value as ApplicationSort)
            }}
            className="rounded-md border border-gray-200 bg-white px-2 py-1 text-xs text-gray-700"
          >
            {SORTS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </div>
      )}
      {shown.map(({ profile, applications }) => (
        <section key={profile.id} aria-label={`Applications of ${profileLabel(profile)}`} className="mb-6">
          <div className="mb-2 flex items-baseline gap-2">
            <h2 className="text-base font-semibold">{profileLabel(profile)}</h2>
            <span className="text-xs text-gray-500">{plural(applications.length, 'application', 'applications')}</span>
            <AppLink href={jobBoardPathFor(profile.id)} className="text-xs font-medium text-blue-700 hover:underline">
              Job board
            </AppLink>
          </div>
          <ul className="space-y-2">
            {sortApplications(applications, sort).map((a) => {
              const accent = a.status ? columnAccent(a.status.kind, a.status.name) : undefined
              return (
                <li key={a.jobId} className="rounded-2xl border border-gray-100 bg-white p-3 shadow-sm">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <AppLink href={jobDetailsPathFor(a.jobId, profile.id)} className="text-sm font-semibold hover:underline">
                      {a.title}
                    </AppLink>
                    {a.company !== '' && <span className="text-xs text-gray-500">{a.company}</span>}
                    {a.status && accent && (
                      <span
                        className="rounded-full px-2 py-0.5 text-[11px] font-semibold"
                        style={{ backgroundColor: accent.soft, color: accent.text }}
                      >
                        {a.status.name}
                      </span>
                    )}
                    {a.waitingForReply && (
                      <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-800">
                        Waiting for a reply
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-gray-600">
                    {protocolText(a, now)} · last activity {activityText(a, now)}
                  </p>
                </li>
              )
            })}
          </ul>
        </section>
      ))}
    </div>
  )
}
