import { useState } from 'react'
import { addJob, DuplicateJobError, type Job } from '../../api'
import { Field } from '../Field/Field'
import { Modal } from '../../Shared/Modal/Modal'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { EMPTY_NEW_JOB, MANUAL_JOB_LIMITS, hasErrors, validateNewJob, type NewJobErrors, type NewJobForm } from './manualJob'

// AddJobModal (step 97) — add a job by hand: one found on LinkedIn, in an email,
// from a friend. It gets everything a crawled job gets (Job Match, Generate CV,
// the company link). Only the title is required. A duplicate is refused with a
// pointer to the job that already exists. See
// plan/ai/tools/career/step-97-add-jobs-manually.md.
export function AddJobModal({
  open,
  onClose,
  onAdded,
  onOpenExisting,
}: {
  open: boolean
  onClose: () => void
  onAdded: (job: Job) => void
  onOpenExisting: (jobId: string) => void
}) {
  const [form, setForm] = useState<NewJobForm>(EMPTY_NEW_JOB)
  const [errors, setErrors] = useState<NewJobErrors>({})
  const [serverError, setServerError] = useState('')
  const [duplicate, setDuplicate] = useState<DuplicateJobError | null>(null)
  const [saving, setSaving] = useState(false)

  function set(key: keyof NewJobForm, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => ({ ...prev, [key]: undefined }))
  }

  function close() {
    setForm(EMPTY_NEW_JOB)
    setErrors({})
    setServerError('')
    setDuplicate(null)
    onClose()
  }

  function submit() {
    const found = validateNewJob(form)
    setErrors(found)
    setServerError('')
    setDuplicate(null)
    if (hasErrors(found)) return
    setSaving(true)
    addJob({
      title: form.title.trim(),
      company: form.company.trim(),
      location: form.location.trim(),
      postedAt: form.postedAt.trim(),
      sourceUrl: form.sourceUrl.trim(),
      description: form.description.trim(),
    })
      .then((job) => {
        setForm(EMPTY_NEW_JOB)
        onAdded(job)
      })
      .catch((err: unknown) => {
        if (err instanceof DuplicateJobError) setDuplicate(err)
        else setServerError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <Modal open={open} title="Add a job" onClose={close}>
      <div className="space-y-3 px-5 py-4">
        <p className="text-xs text-gray-500">
          For a job you found yourself. It can be matched against your profile and used to generate a CV like any other. Only the title is
          required.
        </p>

        {duplicate && (
          <InfoBox
            variant="warning"
            label="Already saved"
            message={`${duplicate.message}.`}
            actions={
              <button
                type="button"
                onClick={() => {
                  const id = duplicate.existingJobId
                  close()
                  onOpenExisting(id)
                }}
                className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs text-orange-800 hover:bg-orange-100"
              >
                Open it
              </button>
            }
          />
        )}
        {serverError && <InfoBox variant="error" message={serverError} />}

        <FieldWithError error={errors.title}>
          <Field
            label="Title"
            value={form.title}
            onChange={(v) => {
              set('title', v)
            }}
          />
        </FieldWithError>
        <div className="grid grid-cols-2 gap-3">
          <FieldWithError error={errors.company}>
            <Field
              label="Company"
              value={form.company}
              onChange={(v) => {
                set('company', v)
              }}
            />
          </FieldWithError>
          <FieldWithError error={errors.location}>
            <Field
              label="Location"
              value={form.location}
              onChange={(v) => {
                set('location', v)
              }}
            />
          </FieldWithError>
        </div>
        <FieldWithError error={errors.sourceUrl}>
          <Field
            label="Link to the posting (optional)"
            value={form.sourceUrl}
            onChange={(v) => {
              set('sourceUrl', v)
            }}
          />
        </FieldWithError>
        <FieldWithError error={errors.postedAt}>
          <Field
            label="Posted on (optional)"
            type="date"
            value={form.postedAt}
            onChange={(v) => {
              set('postedAt', v)
            }}
          />
        </FieldWithError>
        <FieldWithError error={errors.description}>
          <label className="block text-xs font-medium text-gray-500">
            Description (optional) — the text the AI matches your profile against
            <textarea
              value={form.description}
              rows={7}
              maxLength={MANUAL_JOB_LIMITS.description * 2}
              onChange={(e) => {
                set('description', e.target.value)
              }}
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
          </label>
        </FieldWithError>

        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={close}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={saving}
            className="rounded-md bg-gray-800 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-900 disabled:opacity-50"
          >
            {saving ? 'Adding…' : 'Add job'}
          </button>
        </div>
      </div>
    </Modal>
  )
}

function FieldWithError({ error, children }: { error?: string; children: React.ReactNode }) {
  return (
    <div>
      {children}
      {error && (
        <p role="alert" className="mt-1 text-xs text-red-700">
          {error}
        </p>
      )}
    </div>
  )
}
