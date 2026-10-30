import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { addRepository, fetchRepositories, refreshRepository, removeRepository, type ToolRepository } from '../../../api/toolRepositories'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { errorText } from './repositoryErrors'
import {
  addLinkProblem,
  canRemove,
  countsCell,
  formatFetched,
  nativeLabel,
  needsFirstLoad,
  repositoryPath,
  repositoryStatusLabel,
  repositoryStatusVariant,
} from './repositoryDisplay'

// Admin list of tool repositories (api/src/toolrepository): add a link to a
// repository file, then open one on its own page (RepositoryPage) to see its
// tools. See plan/ai/tools/repository/tool-repositories.md.
export function RepositoriesPage() {
  const [repositories, setRepositories] = useState<ToolRepository[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [link, setLink] = useState('')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [rowError, setRowError] = useState<string | null>(null)
  const { confirm, dialog } = useConfirm()

  // Repositories already loaded once automatically; a failed first load is not
  // retried in a loop, the user's Refresh does that.
  const autoLoaded = useRef(new Set<string>())

  const load = useCallback(async () => {
    try {
      let list = await fetchRepositories()
      const fresh = list.filter((r) => needsFirstLoad(r) && !autoLoaded.current.has(r.id))
      if (fresh.length > 0) {
        setRepositories(list)
        await Promise.all(
          fresh.map(async (r) => {
            autoLoaded.current.add(r.id)
            try {
              await refreshRepository(r.id)
            } catch {
              // shown as "Refresh failed" on the next listing; nothing else to do
            }
          }),
        )
        list = await fetchRepositories()
      }
      setRepositories(list)
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

  const handleRefresh = async (repository: ToolRepository) => {
    setBusyId(repository.id)
    setRowError(null)
    try {
      await refreshRepository(repository.id)
      await load()
    } catch (err) {
      setRowError(errorText(err, 'Failed to refresh.'))
    } finally {
      setBusyId(null)
    }
  }

  const handleRemove = async (repository: ToolRepository) => {
    const ok = await confirm({
      title: 'Remove repository',
      message: `Remove "${repository.name}"? Tools already installed from it stay installed.`,
      confirmLabel: 'Remove',
      danger: true,
    })
    if (!ok) return
    setBusyId(repository.id)
    setRowError(null)
    try {
      await removeRepository(repository.id)
      await load()
    } catch (err) {
      setRowError(errorText(err, 'Failed to remove.'))
    } finally {
      setBusyId(null)
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
    <div className="max-w-6xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Repositories</h1>
        <p className="mt-1 text-sm text-gray-500">
          A repository is a link to a JSON file that lists tools. Add one, then open it to see which of its tools you have, which are up to
          date and which can be updated. Nothing installs or updates by itself.
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

      {rowError && (
        <p role="alert" className="text-sm text-red-600">
          {rowError}
        </p>
      )}

      <div className="rounded-lg border border-gray-200">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="text-xs uppercase text-gray-400">
              <th className="px-4 py-2 font-medium">Repository</th>
              <th className="px-4 py-2 font-medium">Tools</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Last checked</th>
              <th className="px-4 py-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {repositories.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-6 text-center text-gray-400">
                  No repositories added.
                </td>
              </tr>
            )}
            {repositories.map((repository) => (
              <tr key={repository.id} className="border-t border-gray-100 align-top">
                <td className="px-4 py-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Link to={repositoryPath(repository.id)} className="font-medium text-gray-900 hover:underline">
                      {repository.name}
                    </Link>
                    {repository.isNative && <Pill variant="green">{nativeLabel}</Pill>}
                    {repository.isLocal && <Pill outline>Local network</Pill>}
                  </div>
                  <div className="break-all font-mono text-xs text-gray-500">{repository.url}</div>
                </td>
                <td className="px-4 py-3 text-gray-700">{countsCell(repository.counts)}</td>
                <td className="px-4 py-3">
                  <Pill variant={repositoryStatusVariant(repository.lastStatus)} title={repository.lastError || undefined}>
                    {repositoryStatusLabel(repository.lastStatus)}
                  </Pill>
                </td>
                <td className="px-4 py-3 text-gray-500">{formatFetched(repository.lastFetchedAt)}</td>
                <td className="px-4 py-3">
                  <div className="flex justify-end gap-2">
                    <Link
                      to={repositoryPath(repository.id)}
                      className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                    >
                      Open
                    </Link>
                    <button
                      type="button"
                      disabled={busyId === repository.id}
                      onClick={() => {
                        void handleRefresh(repository)
                      }}
                      className="rounded-md border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                    >
                      Refresh
                    </button>
                    {canRemove(repository) && (
                      <button
                        type="button"
                        disabled={busyId === repository.id}
                        onClick={() => {
                          void handleRemove(repository)
                        }}
                        className="rounded-md border border-red-200 px-3 py-1 text-xs text-red-600 hover:bg-red-50 disabled:opacity-50"
                      >
                        Remove
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {dialog}
    </div>
  )
}
