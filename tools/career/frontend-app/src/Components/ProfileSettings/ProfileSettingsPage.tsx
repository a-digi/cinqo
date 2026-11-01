import { useEffect, useState } from 'react'
import {
  createProfileMatchRule,
  deleteProfileMatchRule,
  fetchJobStatuses,
  fetchProfileMatchRules,
  fetchProfiles,
  reorderProfileMatchRules,
  updateProfileMatchRule,
  type JobStatusInfo,
  type Profile,
  type ProfileMatchRule,
} from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { PlusIcon, TargetIcon, TrashIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PROFILES_PATH, VIEW_PROFILE_PATH, jobBoardPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { Dropdown } from '../Dropdown/Dropdown'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { ApplyPanel } from './ApplyPanel'
import { idsInOrder, moveRule, neverApplies } from './ruleOrder'
import { MAX_PERCENT, MAX_RULES, MIN_PERCENT, percentFromText, ruleProblem, statusChoices, statusLabel } from './rulesView'

// The drag payload's type: a rule id, only ever read back by this page.
const DRAG_TYPE = 'application/x-career-rule'

const PERCENT_ERROR = `Enter a whole number from ${String(MIN_PERCENT)} to ${String(MAX_PERCENT)}.`

// One profile's auto-add settings (/tools/career/profile-settings/:profileId, step 140): rules of the form "a new job that
// matches at least N% is added to this profile's job board with this status". A profile can have several rules, in an
// order the person sets (drag and drop, or the arrows): the first rule of the list that fits a job decides its status
// (step 143). The rules only act on jobs found from now on, and a job already on the board is never moved. See
// plan/ai/tools/career/step-140-profile-job-match-consumer.md and step-143-apply-rules-to-existing-jobs.md.
export function ProfileSettingsPage({ profileId }: { profileId: string }) {
  const [profile, setProfile] = useState<Profile | null>(null)
  const [rules, setRules] = useState<ProfileMatchRule[]>([])
  const [statuses, setStatuses] = useState<JobStatusInfo[]>([])
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  // The "add a rule" form.
  const [percentInput, setPercentInput] = useState('60')
  const [statusId, setStatusId] = useState<string | null>(null)
  // The rule being changed (its own form replaces its row).
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editPercent, setEditPercent] = useState('')
  const [editStatusId, setEditStatusId] = useState<string | null>(null)
  // The rule being dragged and the place it is over (to show where it would land).
  const [dragId, setDragId] = useState<string | null>(null)
  const [overIndex, setOverIndex] = useState<number | null>(null)

  useEffect(() => {
    setLoaded(false)
    setError('')
    Promise.all([fetchProfiles(), fetchProfileMatchRules(profileId), fetchJobStatuses()])
      .then(([profiles, loadedRules, loadedStatuses]) => {
        setProfile(profiles.find((p) => p.id === profileId) ?? null)
        setRules(loadedRules)
        setStatuses(loadedStatuses)
      })
      .catch((err: unknown) => {
        setProfile(null)
        setError(
          err instanceof Error && err.message.includes('(404)') ? 'Profile not found' : err instanceof Error ? err.message : String(err),
        )
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [profileId])

  const choices = statusChoices(statuses, profileId)
  const options = choices.map((s) => ({ value: s.id, label: s.name }))
  const hasOwnStatus = choices.some((s) => s.kind === 'custom')

  // Runs one change, then reads the rules again (the order and the status names come from the server). A refused value
  // shows the server's reason.
  function change(action: () => Promise<unknown>, done?: () => void) {
    setError('')
    setBusy(true)
    action()
      .then(() => fetchProfileMatchRules(profileId))
      .then((loadedRules) => {
        setRules(loadedRules)
        done?.()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  // Moves a rule to another place: the list changes at once and is saved; when the save fails the list is read again, so the
  // page shows what is really stored.
  function reorder(from: number, to: number) {
    const next = moveRule(rules, from, to)
    if (next === rules) return
    setError('')
    setRules(next)
    setBusy(true)
    reorderProfileMatchRules(profileId, idsInOrder(next))
      .then(setRules)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
        fetchProfileMatchRules(profileId)
          .then(setRules)
          .catch(() => undefined)
      })
      .finally(() => {
        setBusy(false)
      })
  }

  function addRule() {
    const percent = percentFromText(percentInput)
    if (percent === null) {
      setError(PERCENT_ERROR)
      return
    }
    if (statusId === null) {
      setError('Choose a status.')
      return
    }
    const problem = ruleProblem(rules, percent)
    if (problem !== '') {
      setError(problem)
      return
    }
    change(
      () => createProfileMatchRule(profileId, percent, statusId),
      () => {
        setPercentInput('60')
        setStatusId(null)
      },
    )
  }

  function startEdit(rule: ProfileMatchRule) {
    setError('')
    setEditingId(rule.id)
    setEditPercent(String(rule.minPercent))
    setEditStatusId(rule.statusId)
  }

  function saveEdit(rule: ProfileMatchRule) {
    const percent = percentFromText(editPercent)
    if (percent === null) {
      setError(PERCENT_ERROR)
      return
    }
    if (editStatusId === null) {
      setError('Choose a status.')
      return
    }
    const problem = ruleProblem(rules, percent, rule.id)
    if (problem !== '') {
      setError(problem)
      return
    }
    change(
      () => updateProfileMatchRule(rule.id, percent, editStatusId),
      () => {
        setEditingId(null)
      },
    )
  }

  const backHref = profile ? `${VIEW_PROFILE_PATH}?id=${encodeURIComponent(profileId)}` : PROFILES_PATH
  const backLink = (
    <AppLink href={backHref} className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline">
      {profile ? `← ${profileLabel(profile)}` : '← All profiles'}
    </AppLink>
  )

  if (!profile) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{loaded ? error || 'Profile not found' : 'Loading…'}</div>
      </div>
    )
  }

  const atLimit = rules.length >= MAX_RULES
  const dead = neverApplies(rules)
  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-3xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<TargetIcon className="h-6 w-6" />}
        title="Job settings"
        description={`What happens to new jobs for ${profileLabel(profile)} by themselves.`}
        pill={{ text: plural(rules.length, 'rule', 'rules') }}
      />

      <section aria-label="Add new jobs to the job board">
        <h2 className="mb-1 text-sm font-semibold text-gray-700">Add new jobs to the job board</h2>
        <p className="mb-3 text-sm text-gray-600">
          A new job that matches this profile at least the percentage of a rule is added to its{' '}
          <AppLink href={jobBoardPathFor(profileId)} className="font-medium text-blue-700 hover:underline">
            job board
          </AppLink>{' '}
          with that rule’s status. The rules are tried from the top: <strong>the first rule that fits a job decides</strong>. Drag a rule,
          or use its arrows, to change the order.
        </p>

        {error !== '' && (
          <p role="alert" className="mb-3 text-sm text-red-700">
            {error}
          </p>
        )}

        {rules.length === 0 ? (
          <p className="mb-4 rounded-3xl border border-gray-100 bg-gray-50 p-6 text-center text-sm text-gray-500">
            No rules yet. New jobs are not added to this profile’s board by themselves.
          </p>
        ) : (
          <ul className="mb-4 space-y-2" aria-label="Rules">
            {rules.map((rule, index) => {
              const blocker = dead.get(rule.id)
              return editingId === rule.id ? (
                <li key={rule.id} className="flex flex-wrap items-center gap-2 rounded-2xl border border-blue-200 bg-white p-3">
                  <PercentBox
                    value={editPercent}
                    label="Percentage of this rule"
                    onChange={setEditPercent}
                    onEnter={() => {
                      saveEdit(rule)
                    }}
                  />
                  <span className="text-sm text-gray-500">→</span>
                  <Dropdown options={options} value={editStatusId} onChange={setEditStatusId} placeholder="Choose a status" />
                  <div className="ml-auto flex gap-2">
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        saveEdit(rule)
                      }}
                      className="rounded-md bg-gray-800 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-900 disabled:opacity-50"
                    >
                      Save
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setEditingId(null)
                        setError('')
                      }}
                      className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
                    >
                      Cancel
                    </button>
                  </div>
                </li>
              ) : (
                <li
                  key={rule.id}
                  draggable={!busy}
                  onDragStart={(e) => {
                    e.dataTransfer.setData(DRAG_TYPE, rule.id)
                    e.dataTransfer.effectAllowed = 'move'
                    setDragId(rule.id)
                  }}
                  onDragOver={(e) => {
                    if (dragId === null) return
                    e.preventDefault()
                    setOverIndex(index)
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    const from = rules.findIndex((r) => r.id === dragId)
                    setDragId(null)
                    setOverIndex(null)
                    if (from >= 0) reorder(from, index)
                  }}
                  onDragEnd={() => {
                    setDragId(null)
                    setOverIndex(null)
                  }}
                  className={`flex flex-wrap items-center gap-3 rounded-2xl border bg-white/70 p-3 ${
                    dragId === rule.id ? 'opacity-40' : ''
                  } ${dragId !== null && overIndex === index && dragId !== rule.id ? 'border-blue-400' : 'border-gray-100'}`}
                >
                  <span className="cursor-grab select-none text-gray-400" aria-hidden="true" title="Drag to change the order">
                    ⋮⋮
                  </span>
                  <span className="w-5 text-right text-xs font-semibold text-gray-400">{index + 1}.</span>
                  <div className="min-w-0 flex-1">
                    <span className="text-sm text-gray-800">
                      At least <strong>{rule.minPercent}%</strong> match →{' '}
                      <strong>{statusLabel(statuses, rule.statusId, rule.statusName)}</strong>
                    </span>
                    {blocker !== undefined && (
                      <p className="mt-0.5 text-xs text-amber-700">
                        Never applies: rule {blocker.place} ({blocker.minPercent}%) fits every job this one would, and it comes first.
                      </p>
                    )}
                  </div>
                  <div className="ml-auto flex items-center gap-2">
                    <button
                      type="button"
                      disabled={busy || index === 0}
                      aria-label={`Move the rule for ${String(rule.minPercent)}% up`}
                      title="Move up"
                      onClick={() => {
                        reorder(index, index - 1)
                      }}
                      className="rounded-full border border-gray-200 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-30"
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      disabled={busy || index === rules.length - 1}
                      aria-label={`Move the rule for ${String(rule.minPercent)}% down`}
                      title="Move down"
                      onClick={() => {
                        reorder(index, index + 1)
                      }}
                      className="rounded-full border border-gray-200 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-30"
                    >
                      ↓
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        startEdit(rule)
                      }}
                      className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      disabled={busy}
                      aria-label={`Delete the rule for ${String(rule.minPercent)}%`}
                      onClick={() => {
                        change(() => deleteProfileMatchRule(rule.id))
                      }}
                      className="flex items-center gap-1 rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
                    >
                      <TrashIcon />
                      Delete
                    </button>
                  </div>
                </li>
              )
            })}
          </ul>
        )}

        <div className="flex flex-wrap items-center gap-2 rounded-2xl border border-gray-100 bg-white/70 p-3">
          <span className="text-sm text-gray-700">At least</span>
          <PercentBox value={percentInput} label="Percentage of the new rule" onChange={setPercentInput} onEnter={addRule} />
          <span className="text-sm text-gray-700">% match →</span>
          <Dropdown options={options} value={statusId} onChange={setStatusId} placeholder="Choose a status" />
          <button
            type="button"
            disabled={busy || atLimit}
            onClick={addRule}
            className="ml-auto flex items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900 disabled:opacity-50"
          >
            <PlusIcon />
            Add rule
          </button>
        </div>
        <p className="mt-2 text-xs text-gray-500">A new rule is added at the bottom of the list.</p>
        {atLimit && <p className="mt-2 text-xs text-gray-500">A profile can have at most {MAX_RULES} rules.</p>}
        {!hasOwnStatus && (
          <p className="mt-2 text-xs text-gray-500">
            Tip: add your own statuses (like “To check”) on the{' '}
            <AppLink href={jobBoardPathFor(profileId)} className="font-medium text-blue-700 hover:underline">
              job board
            </AppLink>
            ; they can be chosen here.
          </p>
        )}

        <ApplyPanel
          profileId={profileId}
          rulesKey={rules.map((r) => `${r.id}:${String(r.minPercent)}:${r.statusId}:${String(r.position)}`).join('|')}
          hasRules={rules.length > 0}
          disabled={busy}
        />

        <ul className="mt-5 list-disc space-y-1 pl-5 text-xs text-gray-500">
          <li>Rules only act on jobs found from now on; jobs that are already there are not touched.</li>
          <li>A job that is already on this profile’s board is never moved by a rule.</li>
          <li>Deleting a status on the board also deletes the rules that use it.</li>
        </ul>
      </section>
    </div>
  )
}

// The percentage box of a rule: text (so a wrong entry can be told what is wrong), Enter submits.
function PercentBox({
  value,
  label,
  onChange,
  onEnter,
}: {
  value: string
  label: string
  onChange: (value: string) => void
  onEnter: () => void
}) {
  return (
    <input
      type="text"
      inputMode="numeric"
      value={value}
      onChange={(e) => {
        onChange(e.target.value)
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') onEnter()
      }}
      aria-label={label}
      className="w-16 rounded-md border border-gray-300 px-2 py-1.5 text-center text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
    />
  )
}
