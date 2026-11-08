import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  fetchInstallPlan,
  fetchRepository,
  installFromRepository,
  installWithDependencies,
  type InstallPlan,
  refreshRepository,
  removeRepository,
  type RepositoryTool,
  type ToolRepositoryDetail,
} from '../../../api/toolRepositories'
import { fetchTools, type Tool } from '../../../api/tools'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { Card } from '../../../Shared/Components/Card/Card'
import { CardHead } from '../../../Shared/Components/Card/CardHead'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'
import { InstallPlanDialog } from './InstallPlanDialog'
import { isSimple, runText } from './installPlanDisplay'
import { RepositoryLink } from './RepositoryLink'
import { DependenciesBlock } from './ToolDependencies'
import { headGradient, themeForTool } from './toolCardTheme'
import { Byline, LicensePill, PublisherBlock, ToolFacts } from './ToolAttribution'
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
  // The installed tools, which the "Needs" blocks of the cards are checked against.
  const [installedTools, setInstalledTools] = useState<Tool[]>([])
  const { confirm, dialog } = useConfirm()
  // The install with the tools it needs: the plan being confirmed, and why it is shown (again).
  const [planDialog, setPlanDialog] = useState<{ tool: RepositoryTool; plan: InstallPlan; notice?: string; working: boolean } | null>(null)

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

  // The installed tools are only needed for the "Needs" blocks; if they cannot be read the cards simply show every requirement as not installed.
  useEffect(() => {
    let cancelled = false
    fetchTools()
      .then((list) => {
        if (!cancelled) setInstalledTools(list)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [])

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
    setBusy(true)
    setError(null)
    let plan: InstallPlan
    try {
      plan = await fetchInstallPlan(id, tool.id)
    } catch (err) {
      setError(errorText(err, 'Failed to check what this needs.'))
      setBusy(false)
      return
    }
    setBusy(false)
    // More than one step, a mark or a problem: the list is shown. One plain step keeps the plain question.
    if (!isSimple(plan)) {
      setPlanDialog({ tool, plan, working: false })
      return
    }
    const ok = await confirm({
      title: updating ? 'Update tool' : 'Install tool',
      message: installConfirmMessage(tool, detail),
      details: <ToolFacts metadata={tool.metadata} />,
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

  const handlePlanConfirm = async () => {
    if (!planDialog) return
    const { tool, plan } = planDialog
    setPlanDialog({ ...planDialog, working: true })
    try {
      const run = await installWithDependencies(id, tool.id, plan.confirm)
      if (run.ok) {
        setPlanDialog(null)
        reloadAfterToolChange()
        return
      }
      if (run.code === 'step_failed') {
        // what was installed stays: show the answer on the page and refresh what is installed
        setPlanDialog(null)
        setError(runText(run))
        await load()
        fetchTools()
          .then(setInstalledTools)
          .catch(() => undefined)
        return
      }
      // blocked or changed: the new plan is shown, to be looked at again
      setPlanDialog({ tool, plan: run.plan ?? plan, notice: runText(run), working: false })
    } catch (err) {
      setPlanDialog(null)
      setError(errorText(err, 'Failed to install.'))
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

  // the tools this repository lists: a missing requirement among them is "available here"
  const offeredHere = new Set(detail.tools.map((t) => t.id))

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
          <p className="mt-1">
            <RepositoryLink url={detail.url} />
          </p>
          {detail.description && <p className="mt-1 text-sm text-gray-600">{detail.description}</p>}
          <PublisherBlock publisher={detail.publisher} />
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
            {detail.tools.map((tool, index) => {
              const theme = themeForTool(tool.id)
              const action = actionFor(tool)
              return (
                <Card
                  key={tool.id}
                  animate
                  className="group"
                  animationDelayMs={Math.min(index, 12) * 200}
                  footer={
                    action ? (
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => {
                          void handleInstall(tool)
                        }}
                        className="w-full rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
                      >
                        {action === 'update' ? `Update to v${tool.currentVersion}` : 'Install'}
                      </button>
                    ) : undefined
                  }
                  media={
                    <CardHead
                      gradient={headGradient(theme.accent)}
                      accent={theme.accent}
                      shape={theme.shape}
                      icon={theme.icon}
                      title={tool.name}
                      topLeft={<Pill variant={installedState(tool).variant}>{installedState(tool).label}</Pill>}
                    />
                  }
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className="truncate font-mono text-xs text-gray-500">{tool.id}</p>
                    <span className="shrink-0 rounded-full bg-gray-900 px-2.5 py-0.5 text-xs font-semibold text-white">
                      v{tool.currentVersion}
                    </span>
                  </div>
                  <Byline metadata={tool.metadata} className="mt-1.5" />
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    <LicensePill license={tool.metadata?.license} />
                    {tool.installedVersion !== '' && <Pill outline>{`Installed v${tool.installedVersion}`}</Pill>}
                    {showStatusPill(tool) && (
                      <Pill variant={statusVariant[tool.status]} title={tool.reason || undefined}>
                        {statusLabel[tool.status]}
                      </Pill>
                    )}
                  </div>

                  <div className="mt-4 space-y-2.5">
                    <DependenciesBlock
                      name={tool.name}
                      requirements={tool.requires.map((r) => ({ slug: r.id, minVersion: r.minVersion, maxVersion: r.maxVersion }))}
                      installed={installedTools}
                      accent={theme.accent}
                      offered={offeredHere}
                    />
                    <div className="rounded-xl bg-gray-50 px-3 py-2.5">
                      <p className="text-[11px] font-semibold uppercase tracking-wide text-gray-400">About</p>
                      {tool.description.trim() === '' ? (
                        <p className="mt-1 text-sm italic text-gray-400">No description.</p>
                      ) : (
                        <p className="mt-1 line-clamp-4 text-sm leading-relaxed text-gray-700" title={tool.description}>
                          {tool.description}
                        </p>
                      )}
                    </div>
                  </div>
                  {tool.status === 'unavailable' && tool.reason && <p className="mt-3 text-xs text-gray-500">{tool.reason}</p>}
                </Card>
              )
            })}
          </div>
        </>
      )}
      {dialog}
      {planDialog && (
        <InstallPlanDialog
          plan={planDialog.plan}
          rootToolId={planDialog.tool.id}
          notice={planDialog.notice}
          working={planDialog.working}
          onConfirm={() => {
            void handlePlanConfirm()
          }}
          onCancel={() => {
            setPlanDialog(null)
          }}
        />
      )}
    </div>
  )
}
