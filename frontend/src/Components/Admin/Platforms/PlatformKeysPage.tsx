import { useEffect, useState } from 'react'
import { fetchPlatforms, fetchPlatformKeys, createPlatformKey, deletePlatformKey, type Platform, type PlatformKey } from '../../../api/platforms'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { AppScopes } from '../../../config/security/scopes'

// Admin-only view of registered AI platforms and their API keys
// (api/src/platform) — see
// plan/ai/platform/step-06-frontend-platform-and-key-admin-ui.md. One
// page for both: platforms have no independent management surface of
// their own (a compile-time registry, step 1), so they only ever
// inform the add-key form's dropdown.
export function PlatformKeysPage() {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [keys, setKeys] = useState<PlatformKey[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const { confirm, dialog } = useConfirm()

  const load = () => {
    setError(null)
    Promise.all([fetchPlatforms(), fetchPlatformKeys()])
      .then(([p, k]) => {
        setPlatforms(p)
        setKeys(k)
      })
      .catch((err) => {
        setError(err instanceof ApiError ? err.message : 'Failed to load platforms and keys.')
      })
  }

  useEffect(() => {
    load()
  }, [])

  const handleDelete = async (key: PlatformKey) => {
    const confirmed = await confirm({
      title: 'Delete API key',
      message: `Delete "${key.label}"? This cannot be undone.`,
    })
    if (!confirmed) return
    setBusyId(key.id)
    try {
      await deletePlatformKey(key.id)
      setKeys((prev) => (prev ? prev.filter((k) => k.id !== key.id) : prev))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete key.')
    } finally {
      setBusyId(null)
    }
  }

  if (error) {
    return <p className="p-6 text-sm text-red-600">{error}</p>
  }

  if (!platforms || !keys) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading platforms…" />
      </div>
    )
  }

  return (
    <div className="max-w-4xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Platforms</h1>
        <p className="mt-1 text-sm text-gray-500">
          AI platform API keys used by conversations. Keys are encrypted at rest and only ever shown masked.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="text-xs uppercase text-gray-400">
              <th className="px-4 py-2 font-medium">Label</th>
              <th className="px-4 py-2 font-medium">Platform</th>
              <th className="px-4 py-2 font-medium">Key</th>
              <th className="px-4 py-2 font-medium">Created</th>
              <th className="px-4 py-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {keys.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-6 text-center text-gray-400">
                  No API keys registered.
                </td>
              </tr>
            )}
            {keys.map((key) => (
              <tr key={key.id} className="border-t border-gray-100">
                <td className="px-4 py-2 text-gray-900">{key.label}</td>
                <td className="px-4 py-2 text-gray-600">{platformName(platforms, key.platform)}</td>
                <td className="px-4 py-2 font-mono text-xs text-gray-700">{key.maskedKey}</td>
                <td className="px-4 py-2 text-gray-600">{new Date(key.createdAt).toLocaleString()}</td>
                <td className="px-4 py-2 text-right">
                  <ScopeGate scopes={[AppScopes.PlatformManage]}>
                    <button
                      type="button"
                      onClick={() => handleDelete(key)}
                      disabled={busyId === key.id}
                      className="text-xs font-medium text-red-600 hover:text-red-800 disabled:opacity-50"
                    >
                      Delete
                    </button>
                  </ScopeGate>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ScopeGate scopes={[AppScopes.PlatformManage]}>
        <AddKeyForm
          platforms={platforms}
          onCreated={(key) => setKeys((prev) => (prev ? [key, ...prev] : [key]))}
          onError={(message) => setError(message)}
        />
      </ScopeGate>
      {dialog}
    </div>
  )
}

function platformName(platforms: Platform[], id: string): string {
  return platforms.find((p) => p.id === id)?.name ?? id
}

function AddKeyForm({
  platforms,
  onCreated,
  onError,
}: {
  platforms: Platform[]
  onCreated: (key: PlatformKey) => void
  onError: (message: string) => void
}) {
  const [label, setLabel] = useState('')
  const [platform, setPlatform] = useState(platforms[0]?.id ?? '')
  const [key, setKey] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const created = await createPlatformKey({ label, platform, key })
      onCreated(created)
      setLabel('')
      setKey('')
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Failed to create key.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="rounded-lg border border-gray-200 p-4 space-y-3">
      <h2 className="text-sm font-semibold text-gray-900">Add API key</h2>
      <div className="flex flex-wrap gap-3">
        <input
          type="text"
          placeholder="Label"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          required
          className="min-w-40 flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
        />
        <select
          value={platform}
          onChange={(e) => setPlatform(e.target.value)}
          required
          className="rounded border border-gray-300 px-3 py-1.5 text-sm"
        >
          {platforms.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <input
          type="password"
          placeholder="API key"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          required
          className="min-w-48 flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
        />
        <button
          type="submit"
          disabled={submitting}
          className="rounded bg-gray-900 px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
        >
          Add key
        </button>
      </div>
    </form>
  )
}
