import { useEffect, useRef, useState } from 'react'
import { inputClass } from '../../Shared/inputClass'
import { MAX_STATUS_NAME } from './jobBoardRules'

// A small form for a status name: used to add a status and to rename one. Enter saves, Escape cancels. The name is
// checked before it is sent (validate says why not, or ''); a refusal from the server shows under the field.
export function StatusNameForm({
  initial = '',
  submitLabel,
  label,
  validate,
  onSubmit,
  onCancel,
}: {
  initial?: string
  submitLabel: string
  label: string
  validate: (name: string) => string
  onSubmit: (name: string) => Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState(initial)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // The field takes the focus when the form opens (with the current name selected when renaming).
  const inputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    inputRef.current?.focus()
    inputRef.current?.select()
  }, [])

  function submit() {
    const problem = validate(name)
    if (problem !== '') {
      setError(problem)
      return
    }
    setSaving(true)
    setError('')
    onSubmit(name.trim()).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : String(err))
      setSaving(false)
    })
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <input
        ref={inputRef}
        aria-label={label}
        value={name}
        maxLength={MAX_STATUS_NAME}
        disabled={saving}
        onChange={(e) => {
          setName(e.target.value)
          setError('')
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onCancel()
        }}
        className={inputClass}
      />
      {error !== '' && (
        <p role="alert" className="text-xs text-red-700">
          {error}
        </p>
      )}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="rounded-md bg-gray-800 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-900 disabled:opacity-50"
        >
          {submitLabel}
        </button>
        <button
          type="button"
          disabled={saving}
          onClick={onCancel}
          className="rounded-md border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}
