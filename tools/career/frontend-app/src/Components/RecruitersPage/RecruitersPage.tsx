import { useEffect, useMemo, useRef, useState } from 'react'
import { fetchRecruiters, updateRecruiter, removeRecruiter, fetchCompanies, type Recruiter, type Company } from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { PlusIcon, UsersIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { percentOrNull, plural } from '../CompaniesPage/companyStats'
import { COMPANIES_PATH, NEW_RECRUITER_PATH, RECRUITERS_PATH, VIEW_COMPANY_PATH, viewRecruiterPathFor } from '../../paths'
import { recruiterName, summarizeRecruiters } from './recruiterStats'
import { FREELANCER_LABEL, NO_COMPANY_LABEL, companyChips, kindToSend, rowKind } from './recruiterCompaniesRules'
import { Switch } from '../../Shared/Switch/Switch'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { useAddressSearch } from '../../Shared/AddressState/useAddressSearch'
import { writeAddress } from '../../Shared/AddressState/writeAddress'
import { buildRecruitersSearch, isCanonicalRecruitersSearch, parseRecruitersUrl } from './recruitersUrlState'

// A recruiter works for zero or more existing companies, never implicit (step 169:
// a company being deleted only drops it from the recruiter, it never deletes them).
// The companies are edited together with the name and the email. The page has the same layout as Portals and
// Companies: header card, an overview on the left, the recruiters on the right,
// "New recruiter" opens its own page. See
// plan/ai/tools/career/step-17-recruiters-frontend.md.
export function RecruitersPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [recruiters, setRecruiters] = useState<Recruiter[]>([])
  // The company filter is whatever the ADDRESS says (step 136): choosing a company writes the address, the address announces
  // itself, and the list follows, so Back, Forward, a reload and a copied link all show the same list.
  const address = useAddressSearch(RECRUITERS_PATH)
  const filter = useMemo(() => parseRecruitersUrl(address.search), [address.search])
  const filterCompanyId = filter.companyId
  const filterFreelancers = filter.freelancer
  // The one value the filter dropdown holds: a company id, the freelancers, or "" for everyone.
  const FREELANCERS_VALUE = 'freelancers'
  const filterValue = filterFreelancers ? FREELANCERS_VALUE : filterCompanyId
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editFirstName, setEditFirstName] = useState('')
  const [editLastName, setEditLastName] = useState('')
  const [editEmail, setEditEmail] = useState('')
  const [editCompanyIds, setEditCompanyIds] = useState<string[]>([])
  const [editFreelancer, setEditFreelancer] = useState(false)
  const [error, setError] = useState('')

  // Loads the recruiters of the company the address names (all of them when it names none); the latest request wins.
  const latestRequest = useRef(0)
  useEffect(() => {
    setError('')
    const request = ++latestRequest.current
    fetchRecruiters(filterCompanyId, filterFreelancers)
      .then((list) => {
        if (request === latestRequest.current) setRecruiters(list)
      })
      .catch((err: unknown) => {
        if (request === latestRequest.current) setError(err instanceof Error ? err.message : String(err))
      })
  }, [filterCompanyId, filterFreelancers])

  useEffect(() => {
    fetchCompanies()
      .then(setCompanies)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  // An address that is not exactly the canonical one (an empty `companyId=`, a foreign parameter) is rewritten as the
  // canonical one, replacing the entry. Runs on every change of the address.
  useEffect(() => {
    if (!isCanonicalRecruitersSearch(window.location.search)) {
      writeAddress(window, RECRUITERS_PATH, buildRecruitersSearch(parseRecruitersUrl(window.location.search)), 'replace')
    }
  }, [address.tick])

  // Choosing a company or the freelancers pushes an entry (Back returns to the earlier filter).
  function handleFilterChange(value: string) {
    const state = value === FREELANCERS_VALUE ? { companyId: '', freelancer: true } : { companyId: value, freelancer: false }
    writeAddress(window, RECRUITERS_PATH, buildRecruitersSearch(state), 'push')
  }

  function startEdit(r: Recruiter) {
    setEditingId(r.id)
    setEditFirstName(r.firstName)
    setEditLastName(r.lastName)
    setEditEmail(r.email)
    setEditCompanyIds(r.companyIds)
    setEditFreelancer(r.freelancer)
  }

  function cancelEdit() {
    setEditingId(null)
    setEditFirstName('')
    setEditLastName('')
    setEditEmail('')
    setEditCompanyIds([])
    setEditFreelancer(false)
  }

  function saveEdit(r: Recruiter) {
    setError('')
    updateRecruiter(r.id, {
      firstName: editFirstName.trim(),
      lastName: editLastName.trim(),
      email: editEmail.trim(),
      ...kindToSend({ freelancer: r.freelancer, companyIds: r.companyIds }, { freelancer: editFreelancer, companyIds: editCompanyIds }),
    })
      // The answer is the whole list; reload the one the address names, so a recruiter whose companies no longer include the
      // filtered one leaves a filtered list.
      .then(() => fetchRecruiters(filterCompanyId, filterFreelancers))
      .then((list) => {
        setRecruiters(list)
        cancelEdit()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    removeRecruiter(id)
      .then(() => {
        setRecruiters((prev) => prev.filter((r) => r.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const summary = summarizeRecruiters(recruiters)
  const withEmailPercent = percentOrNull(summary.withEmail, summary.recruiters)
  const filtered = filterCompanyId !== '' || filterFreelancers
  const filterCompany = companies.find((c) => c.id === filterCompanyId)

  return (
    <div className="career-v-drop career-a-float career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UsersIcon />}
        title="Recruiters"
        description="Recruiter contacts and the companies they work for, or freelancers attached to none. A recruiter can work for several companies; deleting a company never deletes the recruiter."
        // Filtered to one company, the pill names it and links on to that company's page; otherwise it links to the
        // Companies page, the other list the recruiters belong to.
        pill={
          filtered && filterCompany
            ? {
                text: `${plural(summary.recruiters, 'recruiter', 'recruiters')} · ${filterCompany.name}`,
                href: `${VIEW_COMPANY_PATH}?id=${encodeURIComponent(filterCompany.id)}`,
              }
            : { text: plural(summary.recruiters, 'recruiter', 'recruiters'), href: COMPANIES_PATH }
        }
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <div className="mb-3 flex items-end gap-3">
            <Dropdown
              label="Filter"
              placeholder="All recruiters"
              options={[
                { value: '', label: 'All recruiters' },
                { value: FREELANCERS_VALUE, label: FREELANCER_LABEL + 's' },
                ...companies.map((c) => ({ value: c.id, label: c.name })),
              ]}
              value={filterValue}
              onChange={handleFilterChange}
            />
          </div>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {recruiters.length === 0 && !error && (
            <div
              className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              {filterFreelancers
                ? 'No freelancers yet.'
                : filtered
                  ? 'This company has no recruiters yet.'
                  : 'No recruiters yet. Add the first one with “New recruiter”.'}
            </div>
          )}

          <div className="career-stagger space-y-3">
            {recruiters.map((r) => {
              const name = recruiterName(r)
              const chips = companyChips(r.companies)
              const kind = rowKind(r)
              return (
                <div
                  key={r.id}
                  className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  {editingId === r.id ? (
                    <div>
                      <input
                        // Deliberate: entering edit mode should focus the input
                        // immediately, the same convention most inline-rename UIs use.
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus
                        value={editFirstName}
                        onChange={(e) => {
                          setEditFirstName(e.target.value)
                        }}
                        placeholder="First name"
                        className={`mb-2 ${inputClass}`}
                      />
                      <input
                        value={editLastName}
                        onChange={(e) => {
                          setEditLastName(e.target.value)
                        }}
                        placeholder="Last name"
                        className={`mb-2 ${inputClass}`}
                      />
                      <input
                        value={editEmail}
                        onChange={(e) => {
                          setEditEmail(e.target.value)
                        }}
                        placeholder="Email"
                        className={`mb-2 ${inputClass}`}
                      />
                      <div className="mb-2">
                        <Switch
                          checked={editFreelancer}
                          onChange={(checked) => {
                            setEditFreelancer(checked)
                            if (checked) setEditCompanyIds([])
                          }}
                          label="Freelancer, not attached to a company"
                          size="sm"
                        />
                      </div>
                      {!editFreelancer && (
                        <div className="mb-2">
                          <Dropdown
                            multiple
                            searchable
                            label="Companies"
                            placeholder="No company"
                            options={companies.map((c) => ({ value: c.id, label: c.name }))}
                            value={editCompanyIds}
                            onChange={setEditCompanyIds}
                          />
                        </div>
                      )}
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            saveEdit(r)
                          }}
                          className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          onClick={cancelEdit}
                          className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-start gap-3">
                      <span
                        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                        style={{ background: accentFor(name).to }}
                        aria-hidden="true"
                      >
                        {initialOf(name)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold">{name}</div>
                        {r.email && <div className="mt-0.5 truncate text-xs text-gray-500">{r.email}</div>}
                        <div className="mt-2 flex flex-wrap gap-1.5">
                          {kind === 'freelancer' ? (
                            <span
                              className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                              style={{ background: ACCENTS[4].soft, color: ACCENTS[4].text }}
                            >
                              {FREELANCER_LABEL}
                            </span>
                          ) : kind === 'none' ? (
                            <span className="inline-flex rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-500">
                              {NO_COMPANY_LABEL}
                            </span>
                          ) : (
                            chips.map((chip) => {
                              const accent = accentFor(chip.label)
                              return (
                                <span
                                  key={chip.id}
                                  className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                                  style={{ background: accent.soft, color: accent.text }}
                                >
                                  {chip.label}
                                </span>
                              )
                            })
                          )}
                        </div>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <AppLink
                          href={viewRecruiterPathFor(r.id)}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          View
                        </AppLink>
                        <button
                          type="button"
                          onClick={() => {
                            startEdit(r)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleDelete(r.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 career-rail-sticky lg:w-72">
          {
            <button
              type="button"
              onClick={() => {
                window.__cinqoToolBridge.navigate(`${NEW_RECRUITER_PATH}${buildRecruitersSearch(filter)}`)
              }}
              className="mb-4 flex w-full items-center justify-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
            >
              <PlusIcon />
              New recruiter
            </button>
          }
          <SummaryRail
            title={filtered ? 'Overview (filtered)' : 'Overview'}
            segments={[
              { value: summary.withEmail, color: '#16a34a' },
              { value: summary.withoutEmail, color: '#cbd5e1' },
            ]}
            centerValue={withEmailPercent === null ? '–' : `${withEmailPercent}%`}
            centerLabel="have an email"
            rows={[
              { label: 'Recruiters', value: String(summary.recruiters), color: ACCENTS[0].to },
              { label: 'With email', value: String(summary.withEmail), color: '#16a34a' },
              { label: 'Without email', value: String(summary.withoutEmail), color: ACCENTS[2].to, emphasis: summary.withoutEmail > 0 },
              { label: 'Companies', value: String(summary.companies), color: ACCENTS[4].to },
              ...(summary.freelancers > 0 ? [{ label: 'Freelancers', value: String(summary.freelancers), color: ACCENTS[3].to }] : []),
              ...(summary.withoutCompany > 0
                ? [{ label: 'Without a company', value: String(summary.withoutCompany), color: ACCENTS[2].to, emphasis: true }]
                : []),
            ]}
          />
          <div className="mt-8">
            <PageTips page="recruiters" orientation="vertical" />
          </div>
        </aside>
      </div>
    </div>
  )
}
