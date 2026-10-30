import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  fetchRepository,
  installFromRepository,
  refreshRepository,
  removeRepository,
  type RepositoryTool,
  type ToolRepositoryDetail,
} from '../../../api/toolRepositories'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { Card } from '../../../Shared/Components/Card/Card'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'
import { colorForTool } from './toolCardColors'
import { errorText } from './repositoryErrors'
import {
  actionFor,
  canRemove,
  formatFetched,
  installConfirmMessage,
  installedState,
  nativeLabel,
  needsFirstLoad,
  repositoryStatusLabel,
  repositoryStatusVariant,
  showStatusPill,
  statusLabel,
  statusVariant,
  summaryText,
} from './repositoryDisplay'

const listPath = '/admin/tool-repositories'

// One repository on its own page: its details and, as cards (the shared Card,
// like the Tools page), every tool it lists with its status and an Install or
// Update button behind a confirmation. See
// plan/ai/tools/repository/tool-repositories.md.
export function RepositoryPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [detail, setDetail] = useState<ToolRepositoryDetail | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const { confirm, dialog } = useConfirm()

  // A never-fetched repository (the built-in one, opened directly) is loaded once.
  const autoLoaded = useRef(false)

  const load = useCallback(async () => {
    try {
      let result = await fetchRepository(id)
      if (needsFirstLoad(result) && !autoLoaded.current) {
        autoLoaded.current = true
        setDetail(result)
        try {
          await refreshRepository(id)
          result = await fetchRepository(id)
        } catch {
          // shown as "Refresh failed" below; the user's Refresh retries
        }
      }
      setDetail(result)
      setLoadError(null)
    } catch (err) {
      setLoadError(errorText(err, 'Failed to load this repository.'))
    }
  }, [id])

  useEffect(() => {
    void load()
  }, [load])

  const handleRefresh = async () => {
    setBusy(true)
    setError(null)
    try {
      await refreshRepository(id)
      await load()
    } catch (err) {
      setError(errorText(err, 'Failed to refresh.'))
    } finally {
      setBusy(false)
    }
  }

  const handleRemove = async () => {
    if (!detail) return
    const ok = await confirm({
      title: 'Remove repository',
      message: `Remove "${detail.name}"? Tools already installed from it stay installed.`,
      confirmLabel: 'Remove',
      danger: true,
    })
    if (!ok) return
    setBusy(true)
    try {
      await removeRepository(id)
      void navigate(listPath)
    } catch (err) {
      setError(errorText(err, 'Failed to remove.'))
      setBusy(false)
    }
  }

  const handleInstall = async (tool: RepositoryTool) => {
    if (!detail) return
    const updating = tool.installedVersion !== ''
    const ok = await confirm({
      title: updating ? 'Update tool' : 'Install tool',
      message: installConfirmMessage(tool, detail),
      confirmLabel: updating ? 'Update' : 'Install',
    })
    if (!ok) return
    setBusy(true)
    setError(null)
    try {
      await installFromRepository(id, tool.id)
      reloadAfterToolChange()
    } catch (err) {
      setError(errorText(err, 'Failed to install.'))
      setBusy(false)
    }
  }

  const back = (
    <Link to={listPath} className="text-sm text-gray-500 hover:text-gray-900 hover:underline">
      ← Repositories
    </Link>
  )

  if (loadError) {
    return (
      <div className="space-y-3 p-6">
        {back}
        <p className="text-sm text-red-600">{loadError}</p>
      </div>
    )
  }

  if (!detail) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading repository…" />
      </div>
    )
  }

  return (
    <div className="max-w-6xl space-y-6 p-6">
      {back}

      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-xl font-semibold text-gray-900">{detail.name}</h1>
            <Pill variant={repositoryStatusVariant(detail.lastStatus)} title={detail.lastError || undefined}>
              {repositoryStatusLabel(detail.lastStatus)}
            </Pill>
            {detail.isNative && <Pill variant="green">{nativeLabel}</Pill>}
            {detail.isLocal && <Pill outline>Local network</Pill>}
          </div>
          <p className="mt-1 break-all font-mono text-xs text-gray-500">{detail.url}</p>
          {detail.description && <p className="mt-1 text-sm text-gray-600">{detail.description}</p>}
          <p className="mt-1 text-xs text-gray-400">Last checked: {formatFetched(detail.lastFetchedAt)}</p>
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
          {canRemove(detail) && (
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
          )}
        </div>
      </div>

      {detail.lastStatus === 'error' && (
        <p role="alert" className="rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">
          The last refresh failed: {detail.lastError} The tools below are from the last file that loaded.
        </p>
      )}
      {error && (
        <p role="alert" className="rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <p className="text-sm text-gray-600">{summaryText(detail.counts)}</p>

      {detail.tools.length > 0 && (
        <>
          {/* A uniquely-named grid class, for the same reason as the Tools page:
              an installed tool's own globally-injected Tailwind stylesheet can
              redeclare a bare `.grid-cols-*` and collapse a plain Tailwind grid. */}
          <style>{`
            .repository-card-grid { display: grid; grid-template-columns: 1fr; gap: 1rem; }
            @media (min-width: 768px) {
              .repository-card-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
            }
          `}</style>
          <div className="repository-card-grid">
            {detail.tools.map((tool) => {
              const color = colorForTool(tool.id)
              const action = actionFor(tool)
              return (
                <Card
                  key={tool.id}
                  media={
                    <div
                      className="relative flex h-28 items-center justify-center px-12 text-center"
                      style={{ background: color.background, color: color.color }}
                    >
                      <h3 className="truncate text-lg font-bold">{tool.name}</h3>
                      <div className="absolute left-3 top-3">
                        <Pill variant={installedState(tool).variant}>{installedState(tool).label}</Pill>
                      </div>
                    </div>
                  }
                >
                  <div className="flex items-start justify-between gap-2">
                    <p className="truncate font-mono text-xs text-gray-500">{tool.id}</p>
                    <span className="shrink-0 text-sm font-semibold text-gray-900">v{tool.currentVersion}</span>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {tool.installedVersion !== '' && <Pill outline>{`Installed v${tool.installedVersion}`}</Pill>}
                    {showStatusPill(tool) && (
                      <Pill variant={statusVariant[tool.status]} title={tool.reason || undefined}>
                        {statusLabel[tool.status]}
                      </Pill>
                    )}
                  </div>
                  {tool.description && <p className="mt-3 text-sm text-gray-600">{tool.description}</p>}
                  {tool.status === 'unavailable' && tool.reason && <p className="mt-3 text-xs text-gray-500">{tool.reason}</p>}
                  {action && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        void handleInstall(tool)
                      }}
                      className="mt-4 w-full rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
                    >
                      {action === 'update' ? `Update to v${tool.currentVersion}` : 'Install'}
                    </button>
                  )}
                </Card>
              )
            })}
          </div>
        </>
      )}
      {dialog}
    </div>
  )
}
