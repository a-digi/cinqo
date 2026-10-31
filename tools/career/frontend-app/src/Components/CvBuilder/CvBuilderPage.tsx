import { useEffect, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvPersonaDefaults,
  fetchCvPhotoStatus,
  fetchCvDocument,
  fetchJob,
  linkJobCv,
  createCvDocument,
  previewCvDocument,
  type CvData,
  type CvTemplate,
  type Job,
} from '../../api'
import { PersonaStep } from './PersonaStep'
import { TemplateStep } from './TemplateStep'
import { PersonalDetailsStep } from './PersonalDetailsStep'
import { SkillsStep } from './SkillsStep'
import { ExperienceStep } from './ExperienceStep'
import { TitleAndReviewStep } from './TitleAndReviewStep'
import { StepIndicator } from './StepIndicator'
import { effectiveChoice, photoToSend, type CvPhoto } from './cvPhotoRules'
import { JOBS_PATH, attachFailedMessage, backLabel, bannerText, defaultCvTitle, jobIdFromSearch } from './cvForJob'

import { cvBuilderPathFor, cvDocumentsPathFor } from '../../paths'

const CV_DOCUMENTS_PATH = '/tools/career/cv-documents'

const STEPS = [
  { label: 'Persona' },
  { label: 'Template' },
  { label: 'Personal Details' },
  { label: 'Skills' },
  { label: 'Experience' },
  { label: 'Title & Review' },
] as const

// CvBuilderPage — its own dedicated route (per your own instruction),
// separate from the CV Documents list page, gathering CvData across a
// fixed 6-step wizard rather than one single screen with a persistent
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
export function CvBuilderPage({ personaIdFromPath }: { personaIdFromPath?: string } = {}) {
  const params = new URLSearchParams(window.location.search)
  const editingId = params.get('id')
  // The persona a new CV is for: from the address (/tools/career/cv-builder/:personaId) or, for older links,
  // from ?personaId=. Editing a CV takes the persona from the CV itself.
  const initialPersonaId = personaIdFromPath ?? params.get('personaId')
  // "Generate manually" from the Jobs page (step 98): the job this CV is for. Not
  // used when editing an existing document.
  const forJobId = editingId ? null : jobIdFromSearch(window.location.search)

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
  const [previewing, setPreviewing] = useState(false)
  const [forJob, setForJob] = useState<Job | null>(null)
  // Set once the CV exists but attaching it to the job failed, so Generate then
  // only retries the attach instead of creating a second CV.
  const [createdDocumentId, setCreatedDocumentId] = useState<string | null>(null)
  // hasProfilePhoto gates whether the "attach photo?" checkbox even
  // shows (TemplateStep.tsx) — per your own instruction, no
  // photo uploaded means no checkbox at all, not a disabled/unchecked
  // one. attachPhoto defaults true (checked) and resets to true
  // whenever the persona changes or an existing document is loaded —
  // there's no stored record of whether a past document actually used
  // a photo, so edit mode can't restore that; it just reapplies the
  // same "checked by default" rule new CVs get. WHICH photo was used
  // is stored (step 111) and restored below.
  const [hasProfilePhoto, setHasProfilePhoto] = useState(false)
  const [attachPhoto, setAttachPhoto] = useState(true)
  // The profile's photos (the default first) and which one this CV uses; '' means the default.
  // With several photos the choice is sent on preview and create and remembered on the CV, so
  // editing it later shows the same photo (step 111). The attach flag itself is still not stored.
  const [photos, setPhotos] = useState<CvPhoto[]>([])
  const [photoChoice, setPhotoChoice] = useState('')

  useEffect(() => {
    fetchCvTemplates()
      .then(setTemplates)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  useEffect(() => {
    if (!forJobId) return
    fetchJob(forJobId)
      .then((job) => {
        setForJob(job)
        setTitle((current) => (current === '' ? defaultCvTitle(job) : current))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [forJobId])

  useEffect(() => {
    if (!editingId) return
    fetchCvDocument(editingId)
      .then((doc) => {
        setPersonaId(doc.personaId)
        setTemplateId(doc.templateId)
        setTitle(doc.title)
        setCvData(doc.data)
        setFurthestStep(STEPS.length - 1)
        setAttachPhoto(true)
        setPhotoChoice(doc.photoMediaFileId ?? '')
        fetchCvPhotoStatus(doc.personaId)
          .then(({ hasPhoto, photos: profilePhotos }) => {
            setHasProfilePhoto(hasPhoto)
            setPhotos(profilePhotos)
          })
          .catch(() => {
            setHasProfilePhoto(false)
            setPhotos([])
          })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [editingId])

  // Makes a persona the one this CV is for: its fresh defaults and photos replace whatever the later steps held.
  function selectPersona(id: string) {
    setPersonaId(id)
    setError('')
    setAttachPhoto(true)
    setPhotoChoice('')
    setPhotos([])
    fetchCvPersonaDefaults(id)
      .then(setCvData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    fetchCvPhotoStatus(id)
      .then(({ hasPhoto, photos: profilePhotos }) => {
        setHasProfilePhoto(hasPhoto)
        setPhotos(profilePhotos)
      })
      .catch(() => {
        setHasProfilePhoto(false)
        setPhotos([])
      })
  }

  // A persona picked in the list. A new CV lives on its persona's own address, so that is opened (the page loads
  // again with that persona); when the CV is for a job, or an existing one is being edited, the address stays
  // and the persona changes in place.
  function handlePersonaPick(id: string) {
    if (id === personaId) return
    if (!editingId && !forJobId) {
      window.__cinqoToolBridge.navigate(cvBuilderPathFor(id))
      return
    }
    selectPersona(id)
  }

  // A new CV opened for a persona (from its address): load that persona's defaults and photos straight away.
  useEffect(() => {
    if (!editingId && initialPersonaId) selectPersona(initialPersonaId)
    // Once, when the page opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function goBack() {
    setError('')
    setStep((s) => Math.max(0, s - 1))
  }

  function goNext() {
    if (step === 0 && (!personaId || !cvData)) {
      setError('Pick a persona')
      return
    }
    if (step === 1 && !templateId) {
      setError('Pick a template')
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
    window.__cinqoToolBridge.navigate(forJobId ? JOBS_PATH : personaId ? cvDocumentsPathFor(personaId) : CV_DOCUMENTS_PATH)
  }

  // Renders + generates a real, temporary PDF via pdf_tools — same
  // pipeline handleGenerate's own first half uses — but creates
  // nothing permanent (no Media file, no cv_documents row), so the
  // user can see exactly what they're about to create and decide
  // whether to actually commit to it. Opens in a new tab rather than
  // an in-page overlay, per your own instruction. The tab is opened
  // synchronously, still inside the click handler, and only pointed at
  // the real URL once the response arrives — opening it later, inside
  // the .then(), loses the original click's "user gesture" in most
  // browsers and gets silently popup-blocked. Takes an explicit
  // template id (not always the currently-selected one) so each
  // template card's own "Preview" button in step 1 can preview ANY
  // design with the current persona's data, without first requiring
  // the user to switch their actual selection to it. See
  // plan/ai/career/cv-builder/step-04-frontend-cv-builder.md and
  // plan/ai/career/cv-builder/step-07-per-card-template-preview.md.
  function handlePreviewTemplate(previewTemplateId: string) {
    if (!cvData || !previewTemplateId || !personaId) return
    setError('')
    const previewTab = window.open('', '_blank')
    setPreviewing(true)
    previewCvDocument(personaId, previewTemplateId, cvData, attachPhoto, photoToSend(attachPhoto, photos, photoChoice))
      .then(({ previewUrl: url }) => {
        if (previewTab) {
          previewTab.location.href = url
        } else {
          setError('Please allow popups for this site to preview the CV in a new tab')
        }
      })
      .catch((err: unknown) => {
        previewTab?.close()
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
    // For a job: create the CV (unless an earlier try already did), attach it to
    // the job, and return to the Jobs page. Without a job it is the plain wizard.
    const created = createdDocumentId
      ? Promise.resolve(createdDocumentId)
      : createCvDocument(personaId, templateId, title.trim(), cvData, attachPhoto, photoToSend(attachPhoto, photos, photoChoice)).then(
          (doc) => {
            if (forJobId) setCreatedDocumentId(doc.id)
            return doc.id
          },
        )
    created
      .then((documentId) => {
        if (!forJobId) {
          window.__cinqoToolBridge.navigate(cvDocumentsPathFor(personaId))
          return undefined
        }
        return linkJobCv(forJobId, documentId, 'manual')
          .then(() => {
            window.__cinqoToolBridge.navigate(JOBS_PATH)
          })
          .catch((err: unknown) => {
            setError(attachFailedMessage(err instanceof Error ? err.message : String(err)))
          })
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
    <div className="career-v-right career-enter max-w-5xl p-6">
      <button type="button" onClick={handleCancel} className="mb-4 text-sm text-gray-500 underline hover:text-gray-700">
        {backLabel(forJobId !== null)}
      </button>

      <h1 className="mb-4 text-xl font-semibold text-gray-900">{editingId ? 'Edit CV' : 'New CV'}</h1>
      {forJob && (
        <p className="mb-4 rounded-md bg-gray-50 px-3 py-2 text-sm text-gray-700" role="status">
          {bannerText(forJob)}
        </p>
      )}

      <StepIndicator steps={STEPS} currentStep={step} furthestStep={furthestStep} onStepClick={goToStep} />

      {error && <p className="mb-4 text-sm text-red-700">{error}</p>}

      {step === 0 && (
        <PersonaStep
          personaId={personaId}
          onPick={(persona) => {
            handlePersonaPick(persona.id)
          }}
        />
      )}
      {step === 1 && (
        <TemplateStep
          templates={templates}
          templateId={templateId}
          onTemplateChange={setTemplateId}
          hasProfilePhoto={hasProfilePhoto}
          attachPhoto={attachPhoto}
          onAttachPhotoChange={setAttachPhoto}
          photos={photos}
          photoChoice={effectiveChoice(photos, photoChoice)}
          onPhotoChoiceChange={setPhotoChoice}
          canPreview={Boolean(cvData)}
          previewing={previewing}
          onPreviewTemplate={handlePreviewTemplate}
        />
      )}
      {step === 2 && cvData && <PersonalDetailsStep data={cvData} onChange={setCvData} />}
      {step === 3 && cvData && <SkillsStep data={cvData} onChange={setCvData} />}
      {step === 4 && cvData && <ExperienceStep data={cvData} onChange={setCvData} />}
      {step === 5 && cvData && <TitleAndReviewStep title={title} onTitleChange={setTitle} data={cvData} templateName={templateName} />}

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
              onClick={() => {
                handlePreviewTemplate(templateId)
              }}
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
    </div>
  )
}
