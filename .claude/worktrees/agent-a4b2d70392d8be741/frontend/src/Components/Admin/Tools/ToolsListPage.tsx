import { useEffect, useState } from 'react'
import { fetchTools, enableTool, disableTool, deleteTool, type Tool } from '../../../api/tools'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { IconButton } from '../../../Shared/Components/IconButton/IconButton'
import { PowerIcon, TrashIcon } from '../../../Shared/Components/IconButton/icons'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { Card } from '../../../Shared/Components/Card/Card'
import { AppScopes } from '../../../config/security/scopes'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'
import { ToolInstallButton } from './ToolInstallButton'
import { colorForTool } from './toolCardColors'

// Bounds the functions-pill area's own height — every function pill is
// shown now (no cap), so a tool declaring many (Career: ~35) needs a
// scrollable box instead of letting the card grow unbounded tall.
const FUNCTIONS_MAX_HEIGHT = 'max-h-28'

// Admin-only view of installed tools (api/src/tool) — see
// plan/ai/tools/step-08-frontend-admin-ui.md. Every mutating action
// reloads the whole page afterward — the tool registry is append-only
// (step 7), so this is the simplest correct way to reflect a lifecycle
// change, matching coco-mda's own reasoning. Rendered as a card grid
// (Shared/Components/Card), not a table — each card's banner mirrors a
// real-estate-listing layout: a placeholder "photo" area with floating
// circular action buttons top-right and a status ribbon top-left, a
// name/version header row, a slug subtitle, then a row of neutral
// outline stat chips (kind/enabled/function count) before the
// function-name pills themselves.
export function ToolsListPage() {
  const [tools, setTools] = useState<Tool[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busySlug, setBusySlug] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const { confirm, dialog } = useConfirm()

  useEffect(() => {
    let cancelled = false
    fetchTools()
      .then((result) => {
        if (!cancelled) setTools(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load tools.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const handleToggle = async (tool: Tool) => {
    setBusySlug(tool.slug)
    try {
      if (tool.enabled) {
        await disableTool(tool.slug)
      } else {
        await enableTool(tool.slug)
      }
      reloadAfterToolChange()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update tool.')
      setBusySlug(null)
    }
  }

  const handleDelete = async (tool: Tool) => {
    const confirmed = await confirm({
      title: 'Delete tool',
      message: `Delete "${tool.name}"? This removes its files and cannot be undone.`,
    })
    if (!confirmed) return
    setBusySlug(tool.slug)
    try {
      await deleteTool(tool.slug)
      reloadAfterToolChange()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete tool.')
      setBusySlug(null)
    }
  }

  if (error) {
    return <p className="p-6 text-sm text-red-600">{error}</p>
  }

  if (!tools) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading tools…" />
      </div>
    )
  }

  const q = query.trim().toLowerCase()
  const filtered = q ? tools.filter((t) => t.name.toLowerCase().includes(q) || t.slug.toLowerCase().includes(q)) : tools

  return (
    <div className="max-w-6xl space-y-6 p-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Tools</h1>
          <p className="mt-1 text-sm text-gray-500">
            Installed tools — upload a .zip to install a new one. Only enabled tools are usable by the system.
          </p>
        </div>
        <ScopeGate scopes={[AppScopes.ToolManage]}>
          <ToolInstallButton />
        </ScopeGate>
      </div>

      {tools.length > 0 && (
        <input
          type="text"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder="Search tools…"
          className="w-full max-w-xs rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-gray-500 focus:outline-none"
        />
      )}

      {tools.length === 0 && <p className="py-6 text-center text-sm text-gray-400">No tools installed.</p>}
      {tools.length > 0 && filtered.length === 0 && (
        <p className="py-6 text-center text-sm text-gray-400">No tools match your search.</p>
      )}

      {filtered.length > 0 && (
        <>
          {/* A plain Tailwind `grid grid-cols-1 md:grid-cols-3` here would
              share class names with any installed frontend_and_backend
              tool's own, separately-built, globally-injected Tailwind
              stylesheet (each tool's bundle.js injects a <style> tag with
              zero scoping) — confirmed live: Career's own bundle happens
              to redeclare a bare `.grid-cols-1` with no responsive variant,
              and since it's injected after this page's own stylesheet, it
              wins the cascade and silently collapses this grid back to one
              column regardless of viewport width. A uniquely-named class,
              declared right here, can't collide with anything a tool's own
              Tailwind build would ever generate. */}
          <style>{`
            .tools-card-grid { display: grid; grid-template-columns: 1fr; gap: 1rem; }
            @media (min-width: 768px) {
              .tools-card-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
            }
          `}</style>
          <div className="tools-card-grid">
          {filtered.map((tool) => {
            const cardColor = colorForTool(tool.id)
            return (
              <Card
                key={tool.id}
                media={
                  <div
                    className="relative flex h-28 items-center justify-center px-12 text-center"
                    style={{ background: cardColor.background, color: cardColor.color }}
                  >
                    <h3 className="truncate text-lg font-bold">{tool.name}</h3>
                    <div className="absolute left-3 top-3">
                      <Pill variant={statusVariant[tool.status]}>{tool.status}</Pill>
                    </div>
                    <ScopeGate scopes={[AppScopes.ToolManage]}>
                      <div className="absolute right-3 top-3 flex gap-2">
                        <IconButton
                          floating
                          icon={<PowerIcon />}
                          label={tool.enabled ? 'Disable' : 'Enable'}
                          onClick={() => {
                            void handleToggle(tool)
                          }}
                          disabled={busySlug === tool.slug}
                        />
                        <IconButton
                          floating
                          icon={<TrashIcon />}
                          label="Delete"
                          onClick={() => {
                            void handleDelete(tool)
                          }}
                          disabled={busySlug === tool.slug}
                          variant="danger"
                        />
                      </div>
                    </ScopeGate>
                  </div>
                }
              >
                <div className="flex items-start justify-between gap-2">
                  <p className="truncate font-mono text-xs text-gray-500">{tool.slug}</p>
                  <span className="shrink-0 text-sm font-semibold text-gray-900">v{tool.version}</span>
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  <Pill outline>{tool.kind}</Pill>
                  <Pill outline>{tool.enabled ? 'Enabled' : 'Disabled'}</Pill>
                  <Pill outline>
                    {tool.mcpTools.length} function{tool.mcpTools.length === 1 ? '' : 's'}
                  </Pill>
                </div>

                {tool.mcpTools.length > 0 && (
                  <div className={`mt-3 flex flex-wrap gap-1 overflow-y-auto pr-1 ${FUNCTIONS_MAX_HEIGHT}`}>
                    {tool.mcpTools.map((name) => (
                      <Pill key={name}>{name}</Pill>
                    ))}
                  </div>
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

const statusVariant: Record<Tool['status'], 'gray' | 'green' | 'amber' | 'red'> = {
  installed: 'gray',
  starting: 'amber',
  running: 'green',
  stopped: 'gray',
  error: 'red',
}
