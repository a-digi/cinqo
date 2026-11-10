import { useEffect, useState } from 'react'
import { fetchProfileDetails, updateProfileDetails } from '../../../api'
import { inputClass } from '../../../Shared/inputClass'
import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import { profileSummaryPatch } from '../../GoalsPage/goalsRules'

// The summary of the profile (plan 174): every persona of the profile shows it unless it has worded its own. Edited here, once; the
// persona pages say which of their summaries follow it.
export function ProfileSummaryCard({ profileId, onSaved }: { profileId: string; onSaved: () => void }) {
  const [text, setText] = useState<string | null>(null)
  const [saved, setSaved] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)

  useEffect(() => {
    setText(null)
    setError('')
    fetchProfileDetails(profileId)
      .then((d) => {
        setText(d.summary)
        setSaved(d.summary)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [profileId])

  function handleSave() {
    if (text === null) return
    const patch = profileSummaryPatch(text, saved)
    if (patch.summary === undefined) return
    setError('')
    setSaving(true)
    updateProfileDetails(profileId, patch)
      .then((d) => {
        setText(d.summary)
        setSaved(d.summary)
        setJustSaved(true)
        onSaved()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <section
      className="career-enter career-delay-1 mb-6 rounded-3xl border p-5 shadow-sm"
      style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
    >
      <h2 className="mb-1 text-sm font-semibold text-gray-700">Summary of the profile</h2>
      <p className="mb-2 text-xs text-gray-500">
        Every target role below shows this summary unless it has its own wording. The AI reads it; nothing here is a secret.
      </p>
      <textarea
        id="profile-summary"
        aria-label="Summary of the profile"
        value={text ?? ''}
        disabled={text === null}
        onChange={(e) => {
          setJustSaved(false)
          setText(e.target.value)
        }}
        rows={4}
        className={inputClass}
      />
      <div className="min-h-[1.2em] pt-2 text-sm text-red-700">{error}</div>
      <div className="mt-1 flex items-center gap-3">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || text === null}
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save summary'}
        </button>
        {justSaved && <span className="text-sm text-green-700">Saved</span>}
      </div>
    </section>
  )
}
