import { useState } from 'react'
import { createCompany } from '../../api'
import { BuildingIcon, PlusIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { COMPANIES_PATH } from '../../paths'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'

const inputClass = 'w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none'

// The page behind the Companies list's "New company" button. Reachable, but
// menu-less, like the job details and CV builder pages; saving or cancelling
// goes back to the list.
export function NewCompanyPage() {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  function backToList() {
    window.__cinqoToolBridge.navigate(COMPANIES_PATH)
  }

  function handleCreate() {
    const trimmed = name.trim()
    if (!trimmed) {
      setError('Name is required')
      return
    }
    setError('')
    setIsSaving(true)
    createCompany(trimmed, description.trim())
      .then(backToList)
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<BuildingIcon />}
        title="New company"
        description="Add a company to the directory. You can link jobs and recruiters to it afterwards."
      />
      <form
        onSubmit={(e) => {
          e.preventDefault()
          handleCreate()
        }}
        className="rounded-3xl border p-5 shadow-sm"
        style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
      >
        <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="company-name">
          Name
        </label>
        <input
          id="company-name"
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          value={name}
          onChange={(e) => {
            setName(e.target.value)
          }}
          placeholder="Name"
          className={`mb-4 ${inputClass}`}
        />
        <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="company-description">
          Description
        </label>
        <textarea
          id="company-description"
          value={description}
          onChange={(e) => {
            setDescription(e.target.value)
          }}
          placeholder="Description"
          rows={4}
          className={`mb-4 ${inputClass}`}
        />
        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
        <div className="mt-3 flex gap-2">
          <button
            type="submit"
            disabled={isSaving}
            className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
          >
            <PlusIcon />
            Create company
          </button>
          <button
            type="button"
            onClick={backToList}
            className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}
