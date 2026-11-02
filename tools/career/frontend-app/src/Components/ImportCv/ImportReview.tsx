// ImportReview.tsx — the review of one finished analysis: the checklist of what the AI proposed, the choice of an existing persona,
// and Insert (plan 166, step 3; it used to live inside ImportCvPage and was lost on a reload). It works on a run read from the
// server: the proposal is the run's, and every insert writes the full save summary back onto the same run, so the review can be
// left and resumed on the run's own URL until something was saved.
import { useEffect, useMemo, useState } from 'react'
import { updateCVImportRunSaveSummary, type CVImportRun } from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { IMPORT_CV_PATH } from '../../paths'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { insertSelected } from './insertProposal'
import { ImportPipeline } from './ImportPipeline'
import { parseProposal, type CVImportProposal, type ProposalItem } from './parseProposal'
import { pipelineStages } from './pipelineRules'
import { ProposalRow } from './ProposalRow'
import { resolveDuplicateLabels, type DuplicateLabels } from './resolveDuplicateLabels'
import { buildFullSaveSummary, buildInitialChecked, insertState } from './reviewRules'
import { experienceLabel } from './experienceLabel'
import { educationLabel } from './educationInsertRules'

function readProposal(run: CVImportRun): { proposal: CVImportProposal | null; error: string } {
  try {
    return { proposal: parseProposal(run.aiProposal), error: '' }
  } catch (err) {
    return { proposal: null, error: err instanceof Error ? err.message : String(err) }
  }
}

export function ImportReview({ run, onStartOver }: { run: CVImportRun; onStartOver: () => void }) {
  const parsed = useMemo(() => readProposal(run), [run])
  const proposal = parsed.proposal
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

    const effectiveChecked: Record<string, boolean> = {}
    for (const key of Object.keys(checked)) {
      effectiveChecked[key] = checked[key] && !insertedKeys[key]
    }

    const needsExistingPersona = !effectiveChecked.persona && resolvedPersonaId === null && !insertedKeys.persona
    if (needsExistingPersona && !existingPersonaId) {
      setInsertValidationError('Select an existing persona, or check the proposed one above, before inserting.')
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
      result.skills.forEach((s) => {
        if (s.ok) nextInsertedKeys[`skill-${s.index}`] = true
      })
      result.experience.forEach((e) => {
        if (e.ok) nextInsertedKeys[`experience-${e.index}`] = true
      })
      result.education.forEach((e) => {
        if (e.ok) nextInsertedKeys[`education-${e.index}`] = true
      })
      setInsertedKeys(nextInsertedKeys)
      const nextNotes = { ...insertNotes }
      result.education.forEach((e) => {
        if (e.ok) nextNotes[`education-${e.index}`] = e.note ?? ''
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
        apply(`skill-${s.index}`, s)
      })
      result.experience.forEach((e) => {
        apply(`experience-${e.index}`, e)
      })
      result.education.forEach((e) => {
        apply(`education-${e.index}`, e)
      })
      setInsertErrors(nextInsertErrors)

      const summary = buildFullSaveSummary(proposal, nextInsertedKeys, nextInsertErrors, nextNotes)
      updateCVImportRunSaveSummary(run.id, summary).catch(() => {
        // Non-fatal: the insert already succeeded; losing the history record of it must not error the user's own review.
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
          />
          {!checked.profile && !insertedKeys.profile && (
            <p className="mt-2 text-xs text-gray-400">Will attach to your existing profile instead of creating a new one.</p>
          )}
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Personas</h2>
          <ProposalRow
            item={proposal.persona}
            label={[proposal.persona.name, proposal.persona.title].filter(Boolean).join(' — ') || '(no persona proposed)'}
            resolvedDuplicateLabel={resolvedLabelFor(proposal.persona, 'persona')}
            checked={checked.persona}
            onToggle={() => {
              toggle('persona')
            }}
            done={insertedKeys.persona}
            errorMessage={insertErrors.persona}
          />
          {!checked.persona && !insertedKeys.persona && (
            <div className="mt-2">
              <Dropdown
                options={duplicateLabels?.personaList.map((p) => ({ value: p.id, label: p.name })) ?? []}
                value={existingPersonaId}
                onChange={setExistingPersonaId}
                placeholder="Select an existing persona…"
              />
              <p className="mt-1 text-xs text-gray-400">
                Personal Details, Skills, and Experience below will attach to this persona instead of a new one.
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
          />
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Skills</h2>
          {proposal.skills.length === 0 && <p className="text-sm text-gray-400">No skills proposed.</p>}
          <div className="space-y-1">
            {proposal.skills.map((skill, i) => (
              <ProposalRow
                key={i}
                item={skill}
                label={skill.value}
                resolvedDuplicateLabel={resolvedLabelFor(skill, 'skill')}
                checked={checked[`skill-${i}`]}
                onToggle={() => {
                  toggle(`skill-${i}`)
                }}
                done={insertedKeys[`skill-${i}`]}
                errorMessage={insertErrors[`skill-${i}`]}
              />
            ))}
          </div>
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Experience</h2>
          {proposal.experience.length === 0 && <p className="text-sm text-gray-400">No experience proposed.</p>}
          <div className="space-y-1">
            {proposal.experience.map((exp, i) => (
              <ProposalRow
                key={i}
                item={exp}
                label={experienceLabel(exp)}
                resolvedDuplicateLabel={resolvedLabelFor(exp, 'experience')}
                checked={checked[`experience-${i}`]}
                onToggle={() => {
                  toggle(`experience-${i}`)
                }}
                done={insertedKeys[`experience-${i}`]}
                errorMessage={insertErrors[`experience-${i}`]}
              />
            ))}
          </div>
        </section>

        <section className={cardClassName} style={cardStyle}>
          <h2 className="mb-2 text-base font-semibold text-gray-900">Education</h2>
          {proposal.education.length === 0 && <p className="text-sm text-gray-400">No education proposed.</p>}
          <div className="space-y-1">
            {proposal.education.map((edu, i) => (
              <ProposalRow
                key={i}
                item={edu}
                label={educationLabel(edu)}
                resolvedDuplicateLabel={resolvedLabelFor(edu, 'education')}
                checked={checked[`education-${i}`]}
                onToggle={() => {
                  toggle(`education-${i}`)
                }}
                done={insertedKeys[`education-${i}`]}
                doneNote={insertNotes[`education-${i}`]}
                errorMessage={insertErrors[`education-${i}`]}
              />
            ))}
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
