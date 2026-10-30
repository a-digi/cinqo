import { useCallback, useEffect, useState } from 'react'
import {
  addRepository,
  fetchRepositories,
  fetchRepository,
  installFromRepository,
  refreshRepository,
  removeRepository,
  type RepositoryTool,
  type ToolRepository,
  type ToolRepositoryDetail,
} from '../../../api/toolRepositories'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'
import {
  actionFor,
  addLinkProblem,
  formatFetched,
  installConfirmMessage,
  repositoryStatusLabel,
  repositoryStatusVariant,
  statusLabel,
  statusVariant,
  summaryText,
} from './repositoryDisplay'

function errorText(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

// Admin page for tool repositories (api/src/toolrepository): add a link to a
// repository file, see which of its tools are installed, up to date or can be
// updated, and install or update them. Installs go through the same pipeline as
// an upload and reload the page afterwards, like every other tool change. See
// plan/ai/tools/repository/tool-repositories.md.
export function RepositoriesPage() {
  const [repositories, setRepositories] = useState<ToolRepository[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [link, setLink] = useState('')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setRepositories(await fetchRepositories())
      setError(null)
    } catch (err) {
      setError(errorText(err, 'Failed to load repositories.'))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const handleAdd = async (e: React.SyntheticEvent) => {
    e.preventDefault()
    const problem = addLinkProblem(link)
    if (problem) {
      setAddError(problem)
      return
    }
    setAdding(true)
    setAddError(null)
    try {
      await addRepository(link.trim())
      setLink('')
      await load()
    } catch (err) {
      setAddError(errorText(err, 'Failed to add the repository.'))
    } finally {
      setAdding(false)
    }
  }

  if (error) return <p className="p-6 text-sm text-red-600">{error}</p>

  if (!repositories) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading repositories…" />
      </div>
    )
  }

  return (
    <div className="max-w-5xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Repositories</h1>
        <p className="mt-1 text-sm text-gray-500">
          A repository is a link to a JSON file that lists tools. Add one to see which of its tools you have, which are up to date and which
          can be updated. Nothing installs or updates by itself.
        </p>
      </div>

      <form
        onSubmit={(e) => {
          void handleAdd(e)
        }}
        className="space-y-2"
      >
        <div className="flex gap-2">
          <input
            type="text"
            value={link}
            onChange={(e) => {
              setLink(e.target.value)
            }}
            placeholder="https://example.com/cinqo/tools.json"
            aria-label="Repository link"
            disabled={adding}
            className="w-full max-w-xl rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-gray-500 focus:outline-none disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={adding}
            className="rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {adding ? 'Adding…' : 'Add repository'}
          </button>
        </div>
        {addError && (
          <p role="alert" className="max-w-xl text-sm text-red-600">
            {addError}
          </p>
        )}
      </form>

      {repositories.length === 0 && <p className="py-6 text-center text-sm text-gray-400">No repositories added.</p>}

      {repositories.map((repository) => (
        <RepositoryCard key={repository.id} repository={repository} onChanged={load} />
      ))}
    </div>
  )
}

function RepositoryCard({ repository, onChanged }: { repository: ToolRepository; onChanged: () => Promise<void> }) {
  const [detail, setDetail] = useState<ToolRepositoryDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const { confirm, dialog } = useConfirm()

  const loadDetail = useCallback(async () => {
    try {
      setDetail(await fetchRepository(repository.id))
    } catch (err) {
      setError(errorText(err, 'Failed to load this repository.'))
    }
  }, [repository.id])

  useEffect(() => {
    void loadDetail()
  }, [loadDetail, repository.lastFetchedAt])

  const handleRefresh = async () => {
    setBusy(true)
    setError(null)
    try {
      await refreshRepository(repository.id)
      await onChanged()
      await loadDetail()
    } catch (err) {
      setError(errorText(err, 'Failed to refresh.'))
    } finally {
      setBusy(false)
    }
  }

  const handleRemove = async () => {
    const ok = await confirm({
      title: 'Remove repository',
      message: `Remove "${repository.name}"? Tools already installed from it stay installed.`,
      confirmLabel: 'Remove',
      danger: true,
    })
    if (!ok) return
    setBusy(true)
    try {
      await removeRepository(repository.id)
      await onChanged()
    } catch (err) {
      setError(errorText(err, 'Failed to remove.'))
      setBusy(false)
    }
  }

  const handleInstall = async (tool: RepositoryTool) => {
    const ok = await confirm({
      title: tool.installedVersion !== '' ? 'Update tool' : 'Install tool',
      message: installConfirmMessage(tool, repository),
      confirmLabel: tool.installedVersion !== '' ? 'Update' : 'Install',
    })
    if (!ok) return
    setBusy(true)
    setError(null)
    try {
      await installFromRepository(repository.id, tool.id)
      reloadAfterToolChange()
    } catch (err) {
      setError(errorText(err, 'Failed to install.'))
      setBusy(false)
    }
  }

  return (
    <section className="rounded-lg border border-gray-200 bg-white">
      <div className="flex items-start justify-between gap-4 border-b border-gray-100 p-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-base font-semibold text-gray-900">{repository.name}</h2>
            <Pill variant={repositoryStatusVariant(repository.lastStatus)} title={repository.lastError || undefined}>
              {repositoryStatusLabel(repository.lastStatus)}
            </Pill>
            {repository.isLocal && <Pill outline>Local network</Pill>}
          </div>
          <p className="mt-1 break-all font-mono text-xs text-gray-500">{repository.url}</p>
          {repository.description && <p className="mt-1 text-sm text-gray-600">{repository.description}</p>}
          <p className="mt-1 text-xs text-gray-400">Last checked: {formatFetched(repository.lastFetchedAt)}</p>
        </div>
        <div className="flex shrink-0 gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              void handleRefresh()
            }}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Refresh
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              void handleRemove()
            }}
            className="rounded-md border border-red-200 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
          >
            Remove
          </button>
        </div>
      </div>

      {repository.lastStatus === 'error' && (
        <p role="alert" className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-700">
          The last refresh failed: {repository.lastError} The tools below are from the last file that loaded.
        </p>
      )}
      {error && (
        <p role="alert" className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="p-4">
        {!detail && !error && <LoadingSpinner label="Loading tools…" />}
        {detail && (
          <>
            <p className="mb-3 text-sm text-gray-600">{summaryText(detail.counts)}</p>
            {detail.tools.length > 0 && (
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-gray-100 text-xs uppercase text-gray-400">
                    <th className="py-2 pr-3 font-medium">Tool</th>
                    <th className="py-2 pr-3 font-medium">Installed</th>
                    <th className="py-2 pr-3 font-medium">Latest</th>
                    <th className="py-2 pr-3 font-medium">Status</th>
                    <th className="py-2 font-medium" />
                  </tr>
                </thead>
                <tbody>
                  {detail.tools.map((tool) => {
                    const action = actionFor(tool)
                    return (
                      <tr key={tool.id} className="border-b border-gray-50 align-top">
                        <td className="py-2 pr-3">
                          <div className="font-medium text-gray-900">{tool.name}</div>
                          <div className="font-mono text-xs text-gray-400">{tool.id}</div>
                          {tool.description && <div className="mt-0.5 text-xs text-gray-500">{tool.description}</div>}
                        </td>
                        <td className="py-2 pr-3 text-gray-700">{tool.installedVersion || '—'}</td>
                        <td className="py-2 pr-3 text-gray-700">{tool.currentVersion}</td>
                        <td className="py-2 pr-3">
                          <Pill variant={statusVariant[tool.status]} title={tool.reason || undefined}>
                            {statusLabel[tool.status]}
                          </Pill>
                          {tool.status === 'unavailable' && tool.reason && <div className="mt-1 text-xs text-gray-500">{tool.reason}</div>}
                        </td>
                        <td className="py-2 text-right">
                          {action && (
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() => {
                                void handleInstall(tool)
                              }}
                              className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800 disabled:opacity-50"
                            >
                              {action === 'update' ? 'Update' : 'Install'}
                            </button>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </>
        )}
      </div>
      {dialog}
    </section>
  )
}
