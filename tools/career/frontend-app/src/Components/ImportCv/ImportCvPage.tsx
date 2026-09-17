import { useEffect, useRef, useState } from 'react'
import {
  uploadCV,
  fetchCVImportRuns,
  createCVImportRun,
  updateCVImportRunSaveSummary,
  deleteCVImportRun,
  type CVUploadResult,
  type CVImportRun,
} from '../../api'
import { createConversation, sendMessage } from '../../Cinqo/Conversation/conversation'
import { fetchPlatforms, fetchPlatformKeys, type Platform } from '../../Cinqo/Platform/platformRepository'
import { Dropdown } from '../Dropdown/Dropdown'
import { PDFIcon, UploadIcon } from '../../Shared/Icons/icons'
import { buildImportPrompt } from './buildImportPrompt'
import { parseProposal, isInitiallyChecked, type CVImportProposal, type ProposalItem } from './parseProposal'
import { resolveDuplicateLabels, type DuplicateLabels } from './resolveDuplicateLabels'
import { insertSelected, type InsertResult, type ItemResult } from './insertProposal'
import { ProposalRow } from './ProposalRow'
import { ImportRunDetail } from './ImportRunDetail'
import { experienceLabel } from './experienceLabel'

// The 5 areas a CV import populates — Profile/Personas/Personal
// Details/Skills/Experience, matching the real Career entities these
// map onto. See
// plan/ai/tools/career/import-cv/step-04-import-cv-page-and-menu-entry.md.
const AREAS = ['Profile', 'Personas', 'Personal Details', 'Skills', 'Experience'] as const

type PageState = 'upload' | 'analyzing' | 'review' | 'error'

// buildFullSaveSummary reconstructs the InsertResult shape
// cv_import_runs.go's own save_summary_json stores, from this
// component's own accumulated insertedKeys/insertErrors state — not
// from insertSelected's own per-call return value, which only ever
// covers items attempted in THAT call. Takes the two maps as explicit
// params (not read from component state) so a caller can pass the
// just-computed next state before it's actually committed via
// setState — React state updates aren't synchronously readable
// immediately after being requested.
function buildFullSaveSummary(p: CVImportProposal, insertedKeys: Record<string, boolean>, insertErrors: Record<string, string>): InsertResult {
  function itemResult(key: string): ItemResult | undefined {
    if (insertedKeys[key]) return { ok: true }
    if (insertErrors[key]) return { ok: false, error: insertErrors[key] }
    return undefined
  }
  const skills: (ItemResult & { index: number; value: string })[] = []
  p.skills.forEach((s, i) => {
    const r = itemResult(`skill-${i}`)
    if (r) skills.push({ ...r, index: i, value: s.value })
  })
  const experience: (ItemResult & { index: number; label: string })[] = []
  p.experience.forEach((e, i) => {
    const r = itemResult(`experience-${i}`)
    if (r) experience.push({ ...r, index: i, label: experienceLabel(e) })
  })
  return {
    profile: itemResult('profile'),
    persona: itemResult('persona'),
    personalDetails: itemResult('personalDetails'),
    skills,
    experience,
  }
}

function buildInitialChecked(p: CVImportProposal): Record<string, boolean> {
  const checked: Record<string, boolean> = {
    profile: isInitiallyChecked(p.profile),
    persona: isInitiallyChecked(p.persona),
    personalDetails: isInitiallyChecked(p.personalDetails),
  }
  p.skills.forEach((s, i) => {
    checked[`skill-${i}`] = isInitiallyChecked(s)
  })
  p.experience.forEach((e, i) => {
    checked[`experience-${i}`] = isInitiallyChecked(e)
  })
  return checked
}

// Import CV's own page — upload a CV PDF, then an AI conversation
// reads it via pdf_tools and proposes entries for the 5 areas above
// (step 5), reviewed here as a checklist (step 6) before inserting
// (step 7, not yet wired — "Insert" comes next). The conversation
// created here is a normal, visible one (not hidden) — the user gets a
// full, inspectable transcript of why the AI proposed what it
// proposed, per the plan's own decision.
export function ImportCvPage() {
  const [state, setState] = useState<PageState>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [fileName, setFileName] = useState('')
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const [uploadResult, setUploadResult] = useState<CVUploadResult | null>(null)
  const [conversationId, setConversationId] = useState<string | null>(null)
  // The cv_import_runs row for the CURRENTLY in-progress analysis (set
  // once the AI's reply is successfully parsed) — handleInsert persists
  // its own save summary onto this id after every attempt.
  const [currentRunId, setCurrentRunId] = useState<string | null>(null)
  // Import history — every past analysis (AI proposal + eventual save
  // outcome), newest first. Deletable, viewable read-only, and
  // reprocessable (re-runs the AI against the same underlying upload,
  // skipping re-upload) — see plan/ai/media/step-05-career-history.md.
  const [runs, setRuns] = useState<CVImportRun[] | null>(null)
  // Set while the read-only history detail view is open; null closes
  // it. A plain state field, not a route — this page has no router of
  // its own (main.tsx's own top-level menu paths are the only routes
  // this tool declares).
  const [viewingRun, setViewingRun] = useState<CVImportRun | null>(null)
  const [runActionError, setRunActionError] = useState('')
  // The AI's own raw final reply — kept even after a successful parse
  // so the 'error' state can still show it if something later fails
  // (e.g. duplicate-label resolution), and shown collapsed either way
  // as a "how did the AI arrive at this" reference.
  const [rawReply, setRawReply] = useState('')
  const [proposal, setProposal] = useState<CVImportProposal | null>(null)
  const [duplicateLabels, setDuplicateLabels] = useState<DuplicateLabels | null>(null)
  const [checked, setChecked] = useState<Record<string, boolean>>({})

  // Required only when the persona row is unchecked — the gap
  // resolution step 7's own design calls for: Personal Details/Skills/
  // Experience all need a real personaId, so unchecking the proposed
  // persona (skip, or a possible duplicate the user doesn't want to
  // re-create) means picking which existing one to attach to instead.
  // Pre-filled from the AI's own possibleDuplicateOf guess when it
  // named one, otherwise left for the user to pick.
  const [existingPersonaId, setExistingPersonaId] = useState<string | null>(null)
  // Set once resolved (by creation or by falling back to an existing
  // one) so a retry of "Insert" never creates a second profile/persona
  // for the same review session — insertSelected itself skips
  // re-resolving once these are non-null.
  const [resolvedProfileId, setResolvedProfileId] = useState<string | null>(null)
  const [resolvedPersonaId, setResolvedPersonaId] = useState<string | null>(null)
  // Which exact checklist rows have already been successfully
  // inserted — persists across multiple "Insert" clicks (a retry only
  // re-attempts rows that are checked AND not yet in this set), and
  // locks each such row's own checkbox (ProposalRow's own `done` prop).
  const [insertedKeys, setInsertedKeys] = useState<Record<string, boolean>>({})
  // Per-key error from the most recent insert attempt — cleared for
  // any key that succeeds, replaced for one that fails again.
  const [insertErrors, setInsertErrors] = useState<Record<string, string>>({})
  const [inserting, setInserting] = useState(false)
  const [insertValidationError, setInsertValidationError] = useState('')

  // Platform/model selection is silent, not a picker UI (unlike
  // CrawlPanel's own CrawlPlatformPicker) — Import CV is a one-shot,
  // fully automatic flow with no per-run reason to compare platforms.
  // Same "prefer whichever platform already has a key" preference
  // PortalsPage already established; falls back to the first platform
  // if the key list fails to load.
  const [platform, setPlatform] = useState<Platform | null>(null)
  const [platformsLoaded, setPlatformsLoaded] = useState(false)

  // Dropzone state — visual only (the click-to-browse path already
  // worked before; this adds a real drag-and-drop path to match the
  // dropzone's own dashed-border affordance, since offering that look
  // without the behavior would be a little deceptive).
  const [isDragOver, setIsDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  function pickFile(next: File | null) {
    setError('')
    setFile(next)
  }

  useEffect(() => {
    Promise.all([fetchPlatforms(), fetchPlatformKeys().catch(() => [])])
      .then(([list, keys]) => {
        if (list.length > 0) {
          const withKey = list.find((p) => keys.some((k) => k.platform === p.id))
          setPlatform(withKey ?? list[0])
        }
      })
      .catch(() => {
        // Left null — handleUpload's own check surfaces "no AI platform
        // configured" as a real error at the point it actually matters,
        // rather than a page-level error before the user has even
        // chosen a file.
      })
      .finally(() => {
        setPlatformsLoaded(true)
      })
  }, [])

  function loadRuns() {
    fetchCVImportRuns()
      .then(setRuns)
      .catch(() => {
        // Non-fatal, matches the platforms effect's own reasoning above
        // — the upload flow itself doesn't depend on this list, so a
        // failed load just leaves the section empty rather than
        // blocking anything.
        setRuns([])
      })
  }

  useEffect(() => {
    loadRuns()
  }, [])

  // runAnalysis is the shared tail of both a fresh upload
  // (handleUpload) and reprocessing an existing one
  // (handleProcessAgain) — everything from "we have a fileId" onward:
  // create the AI conversation, send the prompt, parse the reply,
  // record a new cv_import_runs row, and enter the review state. A
  // reprocess of an upload whose underlying Media file has since
  // expired surfaces as a normal 'error' state here (pdf_to_markdown's
  // own tool-unavailable text) — no separate "is this still live"
  // check is made beforehand.
  async function runAnalysis(fileId: string, originalFilename: string) {
    setFileName(originalFilename)
    setState('analyzing')
    try {
      if (!platform) {
        throw new Error('No AI platform is configured — ask an admin to add one under Platforms.')
      }
      const model = platform.models.length > 0 ? platform.models[0] : undefined
      const conversation = await createConversation({
        title: `Import CV: ${originalFilename}`,
        platformId: platform.id,
        model,
      })
      setConversationId(conversation.id)

      const result = await sendMessage(conversation.id, buildImportPrompt(fileId))
      setRawReply(result.content)

      const parsed = parseProposal(result.content)
      const labels = await resolveDuplicateLabels()
      const initialChecked = buildInitialChecked(parsed)

      setProposal(parsed)
      setDuplicateLabels(labels)
      setChecked(initialChecked)
      // Pre-fill the "existing persona" picker when the persona row
      // starts unchecked and the AI named a real, still-existing
      // possible duplicate — saves the user re-picking what the AI
      // already identified. Left empty otherwise (the user must pick).
      if (!initialChecked.persona && parsed.persona.possibleDuplicateOf) {
        const guess = parsed.persona.possibleDuplicateOf
        if (labels.personaList.some((p) => p.id === guess)) setExistingPersonaId(guess)
      }

      const runId = await createCVImportRun({
        fileId,
        originalFilename,
        conversationId: conversation.id,
        aiProposal: parsed,
      }).catch(() => null) // non-fatal — the review flow itself doesn't depend on history being recorded
      setCurrentRunId(runId)
      loadRuns()

      setState('review')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setState('error')
    }
  }

  async function handleUpload() {
    if (!file) return
    setUploading(true)
    setError('')

    let upload: CVUploadResult
    try {
      upload = await uploadCV(file)
      setUploadResult(upload)
      setUploading(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setUploading(false)
      return // stays on the 'upload' state — the inline error above already shows it
    }

    // The upload itself succeeded — any failure from here on is a real
    // 'error' state, not an inline retry-in-place message, since the
    // uploaded file (and its capability URL) are already consumed.
    await runAnalysis(upload.fileId, file.name)
  }

  async function handleProcessAgain(run: CVImportRun) {
    setRunActionError('')
    await runAnalysis(run.mediaFileId, run.originalFilename)
  }

  async function handleDeleteRun(run: CVImportRun) {
    setRunActionError('')
    try {
      await deleteCVImportRun(run.id)
      setRuns((prev) => (prev ? prev.filter((r) => r.id !== run.id) : prev))
      if (viewingRun?.id === run.id) setViewingRun(null)
    } catch (err) {
      setRunActionError(err instanceof Error ? err.message : String(err))
    }
  }

  function toggle(key: string) {
    setChecked((prev) => ({ ...prev, [key]: !prev[key] }))
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

      // Computed as plain local values (not via setState's own
      // functional-updater form) because buildFullSaveSummary below
      // needs the just-computed NEXT state immediately — a React state
      // update isn't synchronously readable right after being
      // requested.
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
      setInsertedKeys(nextInsertedKeys)

      const nextInsertErrors = { ...insertErrors }
      const apply = (key: string, r?: { ok: boolean; error?: string }) => {
        if (!r) return
        // '' clears a stale error from a previous failed attempt —
        // ProposalRow only renders errorMessage when truthy, so this
        // reads identically to "no error" without a dynamic delete.
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
      setInsertErrors(nextInsertErrors)

      if (currentRunId) {
        const summary = buildFullSaveSummary(proposal, nextInsertedKeys, nextInsertErrors)
        updateCVImportRunSaveSummary(currentRunId, summary).catch(() => {
          // Non-fatal — the actual insert already succeeded; losing the
          // history record of it shouldn't error the user's own review.
        })
      }
    } finally {
      setInserting(false)
    }
  }

  function startOver() {
    setState('upload')
    setFile(null)
    setFileName('')
    setUploadResult(null)
    setConversationId(null)
    setCurrentRunId(null)
    setRawReply('')
    setProposal(null)
    setDuplicateLabels(null)
    setChecked({})
    setExistingPersonaId(null)
    setResolvedProfileId(null)
    setResolvedPersonaId(null)
    setInsertedKeys({})
    setInsertErrors({})
    setInsertValidationError('')
    setError('')
  }

  function viewTranscript() {
    if (conversationId) window.__cinqoToolBridge.openConversation(conversationId)
  }

  function resolvedLabelFor(item: ProposalItem, kind: 'persona' | 'experience' | 'skill'): string | undefined {
    if (item.possibleDuplicateOf === null) return undefined
    if (kind === 'skill') return item.possibleDuplicateOf
    if (!duplicateLabels) return undefined
    return kind === 'persona' ? duplicateLabels.personas[item.possibleDuplicateOf] : duplicateLabels.experience[item.possibleDuplicateOf]
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Import CV</h1>
      <p className="mb-5 text-sm text-gray-500">
        Upload a CV and let the AI propose Profile, Persona, Personal Details, Skills, and Experience entries from it.
      </p>

      {state === 'upload' && !viewingRun && (
        <section className="rounded-lg border border-gray-200 p-4 shadow-sm">
          <div
            role="button"
            tabIndex={0}
            onClick={() => {
              fileInputRef.current?.click()
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') fileInputRef.current?.click()
            }}
            onDragOver={(e) => {
              e.preventDefault()
              setIsDragOver(true)
            }}
            onDragLeave={() => {
              setIsDragOver(false)
            }}
            onDrop={(e) => {
              e.preventDefault()
              setIsDragOver(false)
              pickFile(e.dataTransfer.files.length > 0 ? e.dataTransfer.files[0] : null)
            }}
            className={`flex cursor-pointer flex-col items-center rounded-md border-2 border-dashed px-6 py-10 text-center transition-colors ${
              isDragOver ? 'border-gray-400 bg-gray-100' : 'border-gray-300 bg-gray-50 hover:border-gray-400 hover:bg-gray-100'
            }`}
          >
            <div className="relative mb-3 h-16 w-16 text-gray-300">
              <PDFIcon className="h-16 w-16" />
              <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full bg-gray-900 text-white">
                <UploadIcon className="h-3.5 w-3.5" />
              </span>
            </div>
            <p className="mb-1 text-sm font-medium text-gray-700">{file ? file.name : 'Click to choose a PDF, or drag one here'}</p>
            <p className="text-xs text-gray-400">PDF up to 10MB</p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".pdf,application/pdf"
              onChange={(e) => {
                pickFile(e.target.files?.[0] ?? null)
              }}
              className="hidden"
            />
          </div>

          <div className="mt-3 min-h-[1.2em] text-sm text-red-700">{error}</div>
          <button
            type="button"
            onClick={() => {
              void handleUpload()
            }}
            disabled={!file || uploading || !platformsLoaded}
            className="mt-1 rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {uploading ? 'Uploading…' : 'Upload'}
          </button>
        </section>
      )}

      {state === 'upload' && !viewingRun && runs !== null && runs.length > 0 && (
        <section className="mt-4 rounded-lg border border-gray-200 p-4 shadow-sm">
          <h2 className="mb-2 text-sm font-semibold text-gray-900">Import history</h2>
          <div className="mb-2 min-h-[1.2em] text-sm text-red-700">{runActionError}</div>
          <ul className="divide-y divide-gray-100">
            {runs.map((run) => (
              <li key={run.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                <div className="min-w-0">
                  <p className="truncate text-gray-700">{run.originalFilename}</p>
                  <p className="text-xs text-gray-400">{new Date(run.createdAt).toLocaleString()}</p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <button
                    type="button"
                    onClick={() => {
                      setViewingRun(run)
                    }}
                    className="text-xs font-medium text-gray-600 underline hover:text-gray-900"
                  >
                    View
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      void handleProcessAgain(run)
                    }}
                    className="text-xs font-medium text-gray-600 underline hover:text-gray-900"
                  >
                    Process again
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      void handleDeleteRun(run)
                    }}
                    className="text-xs font-medium text-red-600 underline hover:text-red-800"
                  >
                    Delete
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {state === 'upload' && viewingRun && (
        <ImportRunDetail
          run={viewingRun}
          onClose={() => {
            setViewingRun(null)
          }}
        />
      )}

      {state === 'analyzing' && (
        <div className="space-y-4">
          <p className="text-sm text-gray-500">
            Analyzing “{fileName}”…{uploadResult && <span className="ml-1 font-mono text-xs text-gray-400">({uploadResult.fileId})</span>}
          </p>
          {conversationId && (
            <p className="text-xs text-gray-400">
              AI conversation:{' '}
              <button type="button" onClick={viewTranscript} className="underline hover:text-gray-600">
                view transcript
              </button>
            </p>
          )}
          {AREAS.map((area) => (
            <section key={area} className="rounded-md border border-gray-200 p-4 shadow-sm">
              <h2 className="text-base font-semibold text-gray-900">{area}</h2>
            </section>
          ))}
        </div>
      )}

      {state === 'review' && proposal && (
        <div className="space-y-4">
          <p className="text-sm text-gray-500">Reviewed “{fileName}”</p>
          {conversationId && (
            <p className="text-xs text-gray-400">
              AI conversation:{' '}
              <button type="button" onClick={viewTranscript} className="underline hover:text-gray-600">
                view transcript
              </button>
            </p>
          )}

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
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

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
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

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
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

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
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

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
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

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
            {insertValidationError && <p className="mb-2 text-sm text-red-700">{insertValidationError}</p>}
            <button
              type="button"
              onClick={() => {
                void handleInsert()
              }}
              disabled={inserting}
              className="rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {inserting ? 'Inserting…' : 'Insert'}
            </button>
          </section>
        </div>
      )}

      {state === 'error' && (
        <section className="rounded-md border border-gray-200 p-4 shadow-sm">
          <p className="mb-3 text-sm text-red-700">{error}</p>
          {rawReply && (
            <details className="mb-3">
              <summary className="cursor-pointer text-xs text-gray-500">Show the AI's raw reply</summary>
              <pre className="mt-2 whitespace-pre-wrap break-words text-xs text-gray-600">{rawReply}</pre>
            </details>
          )}
          <button
            type="button"
            onClick={startOver}
            className="rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            Start over
          </button>
        </section>
      )}
    </div>
  )
}
