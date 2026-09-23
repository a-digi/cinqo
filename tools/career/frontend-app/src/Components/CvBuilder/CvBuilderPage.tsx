import { useEffect, useRef, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvPersonaDefaults,
  fetchCvDocument,
  createCvDocument,
  previewCvDocument,
  type CvData,
  type CvTemplate,
} from '../../api'
import { PersonaAndTemplateStep } from './PersonaAndTemplateStep'
import { PersonalDetailsStep } from './PersonalDetailsStep'
import { SkillsStep } from './SkillsStep'
import { ExperienceStep } from './ExperienceStep'
import { TitleAndReviewStep } from './TitleAndReviewStep'
import { StepIndicator } from './StepIndicator'
import { CvPreviewModal } from './CvPreviewModal'

const CV_DOCUMENTS_PATH = '/tools/career/cv-documents'

const STEPS = [
  { label: 'Persona & Template' },
  { label: 'Personal Details' },
  { label: 'Skills' },
  { label: 'Experience' },
  { label: 'Title & Review' },
] as const

// CvBuilderPage — its own dedicated route (per your own instruction),
// separate from the CV Documents list page, gathering CvData across a
// fixed 5-step wizard rather than one single screen with a persistent
// sidebar. No menu entry of its own — reached only via CvDocumentsPage's
// own "+ New CV" (?personaId=) and "Edit" (?id=) actions, same
// "reachable, but menu-less" convention JobDetailsPage.tsx already
// established for exactly this kind of destination. The id/personaId
// travel as query parameters, not path segments — this tool's own
// router only ever matches a registered route by an EXACT pathname,
// confirmed by reading JobDetailsPage.tsx directly.
//
// Still one single CvData built up across steps, submitted whole in
// ONE POST at the very end (step 5's own Generate) — the wizard is
// purely a client-side data-gathering UX, not a multi-request flow;
// nothing here calls the backend except the initial load (persona
// defaults or an existing document) and the final submit. See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export function CvBuilderPage() {
  const params = new URLSearchParams(window.location.search)
  const editingId = params.get('id')
  const initialPersonaId = params.get('personaId')

  const [step, setStep] = useState(0)
  // The furthest step actually reached — everything up to and
  // including it can be revisited freely (StepIndicator's own
  // clickable steps); anything past it is locked/grayed out, per your
  // own "can't skip ahead to an unfinished step" instruction. Editing
  // an existing document starts this at the LAST index (below) since
  // every step already has real, saved data — nothing about it is
  // actually "unfinished."
  const [furthestStep, setFurthestStep] = useState(0)
  const [personaId, setPersonaId] = useState<string | null>(editingId ? null : initialPersonaId)
  const [templateId, setTemplateId] = useState('')
  const [title, setTitle] = useState('')
  const [cvData, setCvData] = useState<CvData | null>(null)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  const [loading, setLoading] = useState(Boolean(editingId))
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [previewing, setPreviewing] = useState(false)

  // PersonaSwitcher's own mount effect ALWAYS auto-selects a persona
  // from localStorage/first-in-list and fires onChange exactly once,
  // completely ignoring whatever `personaId` prop it was given — that
  // prop only drives the dropdown's own displayed value (confirmed by
  // reading PersonaSwitcher.tsx directly). In edit mode that first,
  // automatic call is pure noise: this page has ALREADY loaded the
  // document's own true personaId/cvData by the time it fires (a real,
  // live-observed bug — editing silently reverted every field back to
  // the persona's current live defaults). Suppressing exactly that one
  // call is safe and sufficient: PersonaSwitcher's mount effect has an
  // empty dependency array, so it only ever calls onChange
  // automatically once per mount — every call after this is a genuine
  // user click on the dropdown, which should behave exactly as
  // documented (refetch fresh defaults, discarding later-step edits).
  const suppressNextPersonaSync = useRef(Boolean(editingId))

  useEffect(() => {
    fetchCvTemplates()
      .then(setTemplates)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  useEffect(() => {
    if (!editingId) return
    fetchCvDocument(editingId)
      .then((doc) => {
        setPersonaId(doc.personaId)
        setTemplateId(doc.templateId)
        setTitle(doc.title)
        setCvData(doc.data)
        setFurthestStep(STEPS.length - 1)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [editingId])

  function handlePersonaChange(id: string) {
    if (suppressNextPersonaSync.current) {
      suppressNextPersonaSync.current = false
      return
    }
    setPersonaId(id)
    setError('')
    fetchCvPersonaDefaults(id)
      .then(setCvData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function goBack() {
    setError('')
    setStep((s) => Math.max(0, s - 1))
  }

  function goNext() {
    if (step === 0 && (!personaId || !templateId || !cvData)) {
      setError('Pick a persona and a template')
      return
    }
    setError('')
    const next = Math.min(STEPS.length - 1, step + 1)
    setStep(next)
    setFurthestStep((f) => Math.max(f, next))
  }

  // StepIndicator's own onStepClick — a locked (not-yet-reached) step
  // renders its button as `disabled`, so this never actually needs to
  // re-check furthestStep itself, but the check stays here too as a
  // real guard, not just a UI-only one.
  function goToStep(index: number) {
    if (index > furthestStep) return
    setError('')
    setStep(index)
  }

  function handleCancel() {
    window.__cinqoToolBridge.navigate(CV_DOCUMENTS_PATH)
  }

  // Renders + generates a real, temporary PDF via pdf_tools — same
  // pipeline handleGenerate's own first half uses — but creates
  // nothing permanent (no Media file, no cv_documents row), so the
  // user can see exactly what they're about to create and decide
  // whether to actually commit to it. See
  // plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
  function handlePreview() {
    if (!cvData || !templateId) return
    setError('')
    setPreviewing(true)
    previewCvDocument(templateId, cvData)
      .then(({ previewUrl: url }) => {
        setPreviewUrl(url)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setPreviewing(false)
      })
  }

  function handleGenerate() {
    if (!personaId || !cvData) return
    if (!title.trim()) {
      setError('Title is required')
      return
    }
    setError('')
    setGenerating(true)
    createCvDocument(personaId, templateId, title.trim(), cvData)
      .then(() => {
        window.__cinqoToolBridge.navigate(CV_DOCUMENTS_PATH)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setGenerating(false)
      })
  }

  if (loading) {
    return <div className="p-6 text-sm text-gray-500">Loading…</div>
  }

  const templateName = templates.find((t) => t.id === templateId)?.name ?? templateId

  return (
    <div className="mx-auto max-w-2xl p-6">
      <button type="button" onClick={handleCancel} className="mb-4 text-sm text-gray-500 underline hover:text-gray-700">
        ← Back to CV Documents
      </button>

      <h1 className="mb-4 text-xl font-semibold text-gray-900">{editingId ? 'Edit CV' : 'New CV'}</h1>

      <StepIndicator steps={STEPS} currentStep={step} furthestStep={furthestStep} onStepClick={goToStep} />

      {error && <p className="mb-4 text-sm text-red-700">{error}</p>}

      {step === 0 && (
        <PersonaAndTemplateStep
          personaId={personaId}
          onPersonaChange={handlePersonaChange}
          templates={templates}
          templateId={templateId}
          onTemplateChange={setTemplateId}
        />
      )}
      {step === 1 && cvData && <PersonalDetailsStep data={cvData} onChange={setCvData} />}
      {step === 2 && cvData && <SkillsStep data={cvData} onChange={setCvData} />}
      {step === 3 && cvData && <ExperienceStep data={cvData} onChange={setCvData} />}
      {step === 4 && cvData && <TitleAndReviewStep title={title} onTitleChange={setTitle} data={cvData} templateName={templateName} />}

      <div className="mt-6 flex items-center justify-between">
        <button
          type="button"
          onClick={goBack}
          disabled={step === 0}
          className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
        >
          Back
        </button>
        <div className="flex gap-2">
          {cvData && templateId && (
            <button
              type="button"
              onClick={handlePreview}
              disabled={previewing}
              className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              {previewing ? 'Preparing preview…' : 'Preview'}
            </button>
          )}
          {step < STEPS.length - 1 ? (
            <button
              type="button"
              onClick={goNext}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800"
            >
              Next
            </button>
          ) : (
            <button
              type="button"
              onClick={handleGenerate}
              disabled={generating || !title.trim()}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {generating ? 'Generating…' : 'Generate'}
            </button>
          )}
        </div>
      </div>

      {previewUrl && (
        <CvPreviewModal
          src={previewUrl}
          title={title || 'CV preview'}
          onClose={() => {
            setPreviewUrl(null)
          }}
        />
      )}
    </div>
  )
}
