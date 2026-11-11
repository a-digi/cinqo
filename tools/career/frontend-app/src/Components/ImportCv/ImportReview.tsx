// ImportReview.tsx — the review of one finished analysis: the checklist of what the AI proposed, the choice of an existing persona,
// and Insert (plan 166, step 3; it used to live inside ImportCvPage and was lost on a reload). It works on a run read from the
// server: the proposal is the run's, and every insert writes the full save summary back onto the same run, so the review can be
// left and resumed on the run's own URL until something was saved.
import { useEffect, useMemo, useRef, useState } from 'react'
import { fetchCVImportRun, SuggestionsChangedError, updateCVImportRunSaveSummary, type CVImportRun } from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { IMPORT_CV_PATH } from '../../paths'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { insertSelected } from './insertProposal'
import { ImportPipeline } from './ImportPipeline'
import { parseProposal, type CVImportProposal, type ProposalItem } from './parseProposal'
import { pipelineStages } from './pipelineRules'
import { ProposalRow } from './ProposalRow'
import { resolveDuplicateLabels, type DuplicateLabels } from './resolveDuplicateLabels'
import {
  buildFullSaveSummary,
  buildInitialChecked,
  describeDiff,
  insertState,
  mergeProposal,
  rowKey,
  type ReviewState,
} from './reviewRules'
import { experienceLabel } from './experienceLabel'
import { educationLabel } from './educationInsertRules'

function readProposal(run: CVImportRun): { proposal: CVImportProposal | null; error: string } {
  try {
    return { proposal: parseProposal(run.aiProposal), error: '' }
  } catch (err) {
    return { proposal: null, error: err instanceof Error ? err.message : String(err) }
  }
}

// How often the review looks at the run again to see whether the AI changed the suggestions in the chat (plan 257).
export const SUGGESTIONS_POLL_MS = 5000

export function ImportReview({ run, onStartOver }: { run: CVImportRun; onStartOver: () => void }) {
  const parsed = useMemo(() => readProposal(run), [run])
  // The suggestions the person is reviewing and their version. They start as the run's and follow the run while the AI changes them in the chat: the page keeps its own copy so that
  // the rows the person ticked and the rows already inserted stay as they are (the page itself never changes the run's own proposal).
  const [shown, setShown] = useState<{ proposal: CVImportProposal | null; revision: string | undefined }>(() => ({
    proposal: parsed.proposal,
    revision: run.revision,
  }))
  const proposal = shown.proposal
  // What the AI changed since the person last looked: one line, and a tag on the rows (by key).
  const [changeNote, setChangeNote] = useState('')
  const [badges, setBadges] = useState<Record<string, 'new' | 'changed'>>({})
  const [duplicateLabels, setDuplicateLabels] = useState<DuplicateLabels | null>(null)
  const [checked, setChecked] = useState<Record<string, boolean>>(() => (proposal ? buildInitialChecked(proposal) : {}))
  // Required only when the persona row is unchecked: Personal Details, Skills and Experience need a real persona, so
  // skipping the proposed one means picking which existing one to attach to (pre-filled from the AI's own guess).
  const [existingPersonaId, setExistingPersonaId] = useState<string | null>(null)
  // Set once resolved so a retry of "Insert" never creates a second profile/persona for the same review.
  const [resolvedProfileId, setResolvedProfileId] = useState<string | null>(null)
  const [resolvedPersonaId, setResolvedPersonaId] = useState<string | null>(null)
  // Which rows were inserted (a retry only re-attempts rows that are checked and not yet in this set) and the last error per row.
  const [insertedKeys, setInsertedKeys] = useState<Record<string, boolean>>({})
  const [insertErrors, setInsertErrors] = useState<Record<string, string>>({})
  // What a row says besides "Inserted" (an education entry the profile already had), by key.
  const [insertNotes, setInsertNotes] = useState<Record<string, string>>({})
  const [inserting, setInserting] = useState(false)
  const [insertValidationError, setInsertValidationError] = useState('')

  // The latest of everything the poll needs, so the timer never works on a stale copy.
  const latest = useRef({ proposal, revision: shown.revision, checked, insertedKeys, insertErrors, insertNotes, inserting })
  latest.current = { proposal, revision: shown.revision, checked, insertedKeys, insertErrors, insertNotes, inserting }

  // Applies a newer version of the suggestions: the person's state is carried over by row (plan 257), and what changed is said once.
  function applyNewer(newProposal: CVImportProposal, revision: string | undefined, state: ReviewState): ReviewState {
    const cur = latest.current.proposal
    if (!cur) return state
    const merged = mergeProposal(cur, newProposal, state)
    setShown({ proposal: newProposal, revision })
    setChecked(merged.state.checked)
    setInsertedKeys(merged.state.insertedKeys)
    setInsertErrors(merged.state.insertErrors)
    setInsertNotes(merged.state.insertNotes)
    const note = describeDiff(merged.diff)
    if (note) {
      setChangeNote(note)
      setBadges((prev) => {
        const next: Record<string, 'new' | 'changed'> = {}
        for (const key of Object.keys(merged.state.checked)) {
          const kept = prev[key] as 'new' | 'changed' | undefined
          if (kept) next[key] = kept
        }
        for (const key of merged.diff.changed) next[key] = 'changed'
        for (const key of merged.diff.added) next[key] = 'new'
        return next
      })
    }
    return merged.state
  }

  useEffect(() => {
    // An older server gives no revision: nothing to compare, nothing to watch.
    if (run.revision === undefined) return
    const timer = setInterval(() => {
      const cur = latest.current
      if (cur.inserting || !cur.proposal || (typeof document !== 'undefined' && document.visibilityState === 'hidden')) return
      fetchCVImportRun(run.id)
        .then((fresh) => {
          const now = latest.current
          if (fresh?.revision === undefined || fresh.revision === now.revision || now.inserting) return
          let next: CVImportProposal
          try {
            next = parseProposal(fresh.aiProposal)
          } catch {
            return
          }
          applyNewer(next, fresh.revision, {
            checked: now.checked,
            insertedKeys: now.insertedKeys,
            insertErrors: now.insertErrors,
            insertNotes: now.insertNotes,
          })
        })
        .catch(() => undefined)
    }, SUGGESTIONS_POLL_MS)
    return () => {
      clearInterval(timer)
    }
    // the timer reads everything through `latest`, so it is made once per run
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [run.id, run.revision === undefined])

  useEffect(() => {
    if (!proposal) return
    let cancelled = false
    resolveDuplicateLabels()
      .then((labels) => {
        if (cancelled) return
        setDuplicateLabels(labels)
        // Pre-fill the existing-persona picker when the persona row starts unchecked and the AI named a real, still existing one.
        if (!buildInitialChecked(proposal).persona && proposal.persona.possibleDuplicateOf) {
          const guess = proposal.persona.possibleDuplicateOf
          if (labels.personaList.some((p) => p.id === guess)) setExistingPersonaId(guess)
        }
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [proposal])

  if (!proposal) {
    return (
      <section className={cardClassName} style={cardStyle}>
        <p className="text-sm text-red-700">This proposal could not be read: {parsed.error}</p>
      </section>
    )
  }

  function toggle(key: string) {
    setChecked((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  function resolvedLabelFor(item: ProposalItem, kind: 'persona' | 'experience' | 'skill' | 'education'): string | undefined {
    if (item.possibleDuplicateOf === null) return undefined
    if (kind === 'skill') return item.possibleDuplicateOf
    if (!duplicateLabels) return undefined
    if (kind === 'persona') return duplicateLabels.personas[item.possibleDuplicateOf]
    if (kind === 'education') return duplicateLabels.education[item.possibleDuplicateOf]
    return duplicateLabels.experience[item.possibleDuplicateOf]
  }

  async function handleInsert() {
    if (!proposal) return
    setInsertValidationError('')

    // insertSelected works by position (`skill-3`); the review keeps its state by the row's key (its id): the two are translated here and back.
    const wanted = (key: string) => (checked[key] ?? false) && !insertedKeys[key]
    const effectiveChecked: Record<string, boolean> = {
      profile: wanted('profile'),
      persona: wanted('persona'),
      personalDetails: wanted('personalDetails'),
    }
    proposal.skills.forEach((r, i) => {
      effectiveChecked[`skill-${i}`] = wanted(rowKey('skill', r, i))
    })
    proposal.experience.forEach((r, i) => {
      effectiveChecked[`experience-${i}`] = wanted(rowKey('experience', r, i))
    })
    proposal.education.forEach((r, i) => {
      effectiveChecked[`education-${i}`] = wanted(rowKey('education', r, i))
    })

    const needsExistingPersona = !effectiveChecked.persona && resolvedPersonaId === null && !insertedKeys.persona
    if (needsExistingPersona && !existingPersonaId) {
      setInsertValidationError('Select an existing target role, or check the proposed one above, before inserting.')
      return
    }

    setInserting(true)
    try {
      const { profileId, personaId, ...result } = await insertSelected(proposal, effectiveChecked, {
        existingPersonaId,
        resolvedProfileId,
        resolvedPersonaId,
      })
      setResolvedProfileId(profileId)
      setResolvedPersonaId(personaId)

      // Plain local values (not setState's updater form): buildFullSaveSummary needs the just-computed next state at once.
      const nextInsertedKeys = { ...insertedKeys }
      if (result.profile?.ok) nextInsertedKeys.profile = true
      if (result.persona?.ok) nextInsertedKeys.persona = true
      if (result.personalDetails?.ok) nextInsertedKeys.personalDetails = true
      const skillKey = (index: number) => rowKey('skill', proposal.skills[index], index)
      const experienceKey = (index: number) => rowKey('experience', proposal.experience[index], index)
      const educationKey = (index: number) => rowKey('education', proposal.education[index], index)
      result.skills.forEach((s) => {
        if (s.ok) nextInsertedKeys[skillKey(s.index)] = true
      })
      result.experience.forEach((e) => {
        if (e.ok) nextInsertedKeys[experienceKey(e.index)] = true
      })
      result.education.forEach((e) => {
        if (e.ok) nextInsertedKeys[educationKey(e.index)] = true
      })
      setInsertedKeys(nextInsertedKeys)
      const nextNotes = { ...insertNotes }
      result.education.forEach((e) => {
        if (e.ok) nextNotes[educationKey(e.index)] = e.note ?? ''
      })
      setInsertNotes(nextNotes)

      const nextInsertErrors = { ...insertErrors }
      const apply = (key: string, r?: { ok: boolean; error?: string }) => {
        if (!r) return
        // '' clears a stale error from a previous attempt (ProposalRow only renders a truthy message).
        nextInsertErrors[key] = r.ok ? '' : (r.error ?? '')
      }
      apply('profile', result.profile)
      apply('persona', result.persona)
      apply('personalDetails', result.personalDetails)
      result.skills.forEach((s) => {
        apply(skillKey(s.index), s)
      })
      result.experience.forEach((e) => {
        apply(experienceKey(e.index), e)
      })
      result.education.forEach((e) => {
        apply(educationKey(e.index), e)
      })
      setInsertErrors(nextInsertErrors)

      const summary = buildFullSaveSummary(proposal, nextInsertedKeys, nextInsertErrors, nextNotes)
      updateCVImportRunSaveSummary(run.id, summary, shown.revision).catch(async (err: unknown) => {
        // The AI changed the suggestions between the person's reading and the save (plan 257): read them again, keep what was inserted by row, and record the summary for the new version.
        // Any other failure is non-fatal: the insert already succeeded; losing the history record of it must not error the user's own review.
        if (!(err instanceof SuggestionsChangedError)) return
        try {
          const fresh = await fetchCVImportRun(run.id)
          if (!fresh) return
          const next = parseProposal(fresh.aiProposal)
          const state = applyNewer(next, fresh.revision, {
            checked,
            insertedKeys: nextInsertedKeys,
            insertErrors: nextInsertErrors,
            insertNotes: nextNotes,
          })
          await updateCVImportRunSaveSummary(
            run.id,
            buildFullSaveSummary(next, state.insertedKeys, state.insertErrors, state.insertNotes),
            fresh.revision,
          )
        } catch {
          // the page shows the new version at its next look; nothing more to do here
        }
      })
    } finally {
      setInserting(false)
    }
  }

  // plan/ai/tools/career/step-167: the Insert button is there while something ticked is still to insert, gone once it is all saved.
  const insert = insertState(checked, insertedKeys, insertErrors, inserting)
  const stages = pipelineStages({
    state: 'review',
    uploadError: false,
    uploading: false,
    inserting,
    insertedCount: Object.values(insertedKeys).filter(Boolean).length,
    insertErrorCount: Object.values(insertErrors).filter(Boolean).length,
  })

  return (
    <div>
      <ImportPipeline stages={stages} onStartOver={onStartOver} />
      {changeNote && (
        <div
          role="status"
          data-testid="suggestions-changed"
          className="mb-4 flex items-start justify-between gap-3 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-900"
        >
          <span>{changeNote}</span>
          <button
            type="button"
            onClick={() => {
              setChangeNote('')
              setBadges({})
            }}
            className="shrink-0 text-xs font-medium underline hover:no-underline"
          >
            OK
          </button>
        </div>
      )}
      <div className="career-stagger space-y-4">
        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Profile</h2>
          <ProposalRow
            item={proposal.profile}
            label={proposal.profile.name || '(no name proposed)'}
            checked={checked.profile}
            onToggle={() => {
              toggle('profile')
            }}
            done={insertedKeys.profile}
            errorMessage={insertErrors.profile}
            badge={badges.profile}
          />
          {!checked.profile && !insertedKeys.profile && (
            <p className="mt-2 text-xs text-gray-400">Will attach to your existing profile instead of creating a new one.</p>
          )}
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Target roles</h2>
          <ProposalRow
            item={proposal.persona}
            label={[proposal.persona.name, proposal.persona.title].filter(Boolean).join(' — ') || '(no target role proposed)'}
            resolvedDuplicateLabel={resolvedLabelFor(proposal.persona, 'persona')}
            checked={checked.persona}
            onToggle={() => {
              toggle('persona')
            }}
            done={insertedKeys.persona}
            errorMessage={insertErrors.persona}
            badge={badges.persona}
          />
          {!checked.persona && !insertedKeys.persona && (
            <div className="mt-2">
              <Dropdown
                options={duplicateLabels?.personaList.map((p) => ({ value: p.id, label: p.name })) ?? []}
                value={existingPersonaId}
                onChange={setExistingPersonaId}
                placeholder="Select an existing target role…"
              />
              <p className="mt-1 text-xs text-gray-400">
                Personal Details, Skills, and Experience below will attach to this target role instead of a new one.
              </p>
            </div>
          )}
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Personal Details</h2>
          <ProposalRow
            item={proposal.personalDetails}
            label={proposal.personalDetails.headline || '(no headline proposed)'}
            checked={checked.personalDetails}
            onToggle={() => {
              toggle('personalDetails')
            }}
            done={insertedKeys.personalDetails}
            errorMessage={insertErrors.personalDetails}
            badge={badges.personalDetails}
          />
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Skills</h2>
          {proposal.skills.length === 0 && <p className="text-sm text-gray-400">No skills proposed.</p>}
          <div className="space-y-1">
            {proposal.skills.map((skill, i) => {
              const key = rowKey('skill', skill, i)
              return (
                <ProposalRow
                  key={key}
                  item={skill}
                  label={skill.value}
                  resolvedDuplicateLabel={resolvedLabelFor(skill, 'skill')}
                  checked={checked[key]}
                  onToggle={() => {
                    toggle(key)
                  }}
                  done={insertedKeys[key]}
                  errorMessage={insertErrors[key]}
                  badge={badges[key]}
                />
              )
            })}
          </div>
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Experience</h2>
          {proposal.experience.length === 0 && <p className="text-sm text-gray-400">No experience proposed.</p>}
          <div className="space-y-1">
            {proposal.experience.map((exp, i) => {
              const key = rowKey('experience', exp, i)
              return (
                <ProposalRow
                  key={key}
                  item={exp}
                  label={experienceLabel(exp)}
                  detail={exp.description}
                  resolvedDuplicateLabel={resolvedLabelFor(exp, 'experience')}
                  checked={checked[key]}
                  onToggle={() => {
                    toggle(key)
                  }}
                  done={insertedKeys[key]}
                  errorMessage={insertErrors[key]}
                  badge={badges[key]}
                />
              )
            })}
          </div>
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Education</h2>
          {proposal.education.length === 0 && <p className="text-sm text-gray-400">No education proposed.</p>}
          <div className="space-y-1">
            {proposal.education.map((edu, i) => {
              const key = rowKey('education', edu, i)
              return (
                <ProposalRow
                  key={key}
                  item={edu}
                  label={educationLabel(edu)}
                  resolvedDuplicateLabel={resolvedLabelFor(edu, 'education')}
                  checked={checked[key]}
                  onToggle={() => {
                    toggle(key)
                  }}
                  done={insertedKeys[key]}
                  doneNote={insertNotes[key]}
                  errorMessage={insertErrors[key]}
                  badge={badges[key]}
                />
              )
            })}
          </div>
        </section>

        <section className={cardClassName} style={cardStyle}>
          {insertValidationError && <p className="mb-2 text-sm text-red-700">{insertValidationError}</p>}
          {insert.view === 'saved' ? (
            <p className="text-sm text-green-700">
              Saved: {insert.insertedCount} {insert.insertedCount === 1 ? 'item was' : 'items were'} added to your profile.{' '}
              <a
                href={IMPORT_CV_PATH}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(IMPORT_CV_PATH)
                }}
                className="text-blue-700 underline hover:text-blue-800"
              >
                Back to the import history
              </a>
            </p>
          ) : (
            <>
              {insert.view === 'disabled' && <p className="mb-2 text-xs text-gray-500">Tick at least one row to insert it.</p>}
              <button
                type="button"
                onClick={() => {
                  void handleInsert()
                }}
                disabled={inserting || insert.view === 'disabled'}
                className="rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
              >
                {inserting ? 'Inserting…' : insert.failedCount > 0 ? 'Insert again' : 'Insert'}
              </button>
            </>
          )}
        </section>
      </div>
    </div>
  )
}
