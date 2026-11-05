import { useState } from 'react'
import type { ApplicationMessageInput, CvDocument, JobRecruiter } from '../../api'
import { Modal } from '../../Shared/Modal/Modal'
import { inputClass } from '../../Shared/inputClass'
import { recruiterLabel } from '../Recruiters/recruiterMapRules'
import { CHANNELS, DIRECTIONS, MAX_BODY, MAX_SHORT_TEXT, formProblem, toInput, type MessageForm } from './protocolRules'

interface MessageModalProps {
  title: string
  initial: MessageForm
  recruiters: readonly JobRecruiter[]
  cvDocuments: readonly CvDocument[]
  // A recruiter or CV the entry names that is no longer in the lists is still offered, so saving does not silently drop it.
  onSave: (input: ApplicationMessageInput) => Promise<void>
  onClose: () => void
}

const label = 'block text-xs font-medium text-gray-500'

// The form of one protocol entry, in a modal. The parent mounts it only while it is open (key per entry), so each opening starts from
// `initial`. The server checks everything again; its reason is shown when it refuses.
export function MessageModal({ title, initial, recruiters, cvDocuments, onSave, onClose }: MessageModalProps) {
  const [form, setForm] = useState<MessageForm>(initial)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  function set<K extends keyof MessageForm>(field: K, value: MessageForm[K]) {
    setForm((prev) => ({ ...prev, [field]: value }))
  }

  const needsLabel = form.channel === 'platform' || form.channel === 'other' || form.channelLabel !== ''
  const recruiterKnown = form.recruiterId === '' || recruiters.some((r) => r.id === form.recruiterId)
  const cvKnown = form.cvDocumentId === '' || cvDocuments.some((c) => c.id === form.cvDocumentId)

  function submit() {
    const problem = formProblem(form)
    if (problem !== '') {
      setError(problem)
      return
    }
    setError('')
    setSaving(true)
    onSave(toInput(form))
      .then(onClose)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
        setSaving(false)
      })
  }

  return (
    <Modal open title={title} onClose={onClose} size="wide">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <label className={label}>
            Direction
            <select
              value={form.direction}
              onChange={(e) => {
                set('direction', e.target.value as MessageForm['direction'])
              }}
              className={`${inputClass} mt-1`}
            >
              {DIRECTIONS.map((d) => (
                <option key={d.value} value={d.value}>
                  {d.label}
                </option>
              ))}
            </select>
          </label>
          <label className={label}>
            Channel
            <select
              value={form.channel}
              onChange={(e) => {
                set('channel', e.target.value as MessageForm['channel'])
              }}
              className={`${inputClass} mt-1`}
            >
              {CHANNELS.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          </label>
          {needsLabel && (
            <label className={label}>
              Platform or app (optional)
              <input
                value={form.channelLabel}
                maxLength={MAX_SHORT_TEXT}
                placeholder="LinkedIn, Signal…"
                onChange={(e) => {
                  set('channelLabel', e.target.value)
                }}
                className={`${inputClass} mt-1`}
              />
            </label>
          )}
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className={label}>
            Date
            <input
              type="date"
              value={form.date}
              onChange={(e) => {
                set('date', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            />
          </label>
          <label className={label}>
            Time (optional)
            <input
              type="time"
              value={form.time}
              onChange={(e) => {
                set('time', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            />
          </label>
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className={label}>
            Recruiter (optional)
            <select
              value={form.recruiterId}
              onChange={(e) => {
                set('recruiterId', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            >
              <option value="">— none —</option>
              {recruiters.map((r) => (
                <option key={r.id} value={r.id}>
                  {recruiterLabel(r)}
                </option>
              ))}
              {!recruiterKnown && <option value={form.recruiterId}>(a recruiter that is no longer mapped)</option>}
            </select>
          </label>
          <label className={label}>
            CV that was sent (optional)
            <select
              value={form.cvDocumentId}
              onChange={(e) => {
                set('cvDocumentId', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            >
              <option value="">— none —</option>
              {cvDocuments.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.title}
                </option>
              ))}
              {!cvKnown && <option value={form.cvDocumentId}>(a CV that no longer exists)</option>}
            </select>
          </label>
          <label className={label}>
            Other side&apos;s name (optional)
            <input
              value={form.counterpartName}
              maxLength={MAX_SHORT_TEXT}
              onChange={(e) => {
                set('counterpartName', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            />
          </label>
          <label className={label}>
            Address, number or handle (optional)
            <input
              value={form.counterpartContact}
              maxLength={MAX_SHORT_TEXT}
              onChange={(e) => {
                set('counterpartContact', e.target.value)
              }}
              className={`${inputClass} mt-1`}
            />
          </label>
        </div>

        <label className={label}>
          Subject (optional)
          <input
            value={form.subject}
            maxLength={MAX_SHORT_TEXT}
            onChange={(e) => {
              set('subject', e.target.value)
            }}
            className={`${inputClass} mt-1`}
          />
        </label>
        <label className={label}>
          Text (optional)
          <textarea
            value={form.body}
            rows={5}
            maxLength={MAX_BODY}
            onChange={(e) => {
              set('body', e.target.value)
            }}
            className={`${inputClass} mt-1`}
          />
        </label>
        <label className={label}>
          Link to the thread (optional)
          <input
            value={form.externalUrl}
            placeholder="https://…"
            onChange={(e) => {
              set('externalUrl', e.target.value)
            }}
            className={`${inputClass} mt-1`}
          />
        </label>

        {error !== '' && (
          <p role="alert" className="text-xs text-red-700">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-700 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
