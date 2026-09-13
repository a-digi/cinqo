import { useEffect, useState } from 'react'
import { fetchTools, enableTool, disableTool, deleteTool, type Tool } from '../../../api/tools'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { AppScopes } from '../../../config/security/scopes'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'
import { ToolInstallButton } from './ToolInstallButton'

// Admin-only view of installed tools (api/src/tool) — see
// plan/ai/tools/step-08-frontend-admin-ui.md. Every mutating action
// reloads the whole page afterward — the tool registry is append-only
// (step 7), so this is the simplest correct way to reflect a lifecycle
// change, matching coco-mda's own reasoning.
export function ToolsListPage() {
  const [tools, setTools] = useState<Tool[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busySlug, setBusySlug] = useState<string | null>(null)
  const { confirm, dialog } = useConfirm()

  useEffect(() => {
    let cancelled = false
    fetchTools()
      .then((result) => {
        if (!cancelled) setTools(result)
      })
      .catch((err) => {
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

  return (
    <div className="max-w-4xl space-y-6 p-6">
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

      <div className="rounded-lg border border-gray-200">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="text-xs uppercase text-gray-400">
              <th className="px-4 py-2 font-medium">Name</th>
              <th className="px-4 py-2 font-medium">Slug</th>
              <th className="px-4 py-2 font-medium">Version</th>
              <th className="px-4 py-2 font-medium">Kind</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Enabled</th>
              <th className="px-4 py-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {tools.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-gray-400">
                  No tools installed.
                </td>
              </tr>
            )}
            {tools.map((tool) => (
              <tr key={tool.id} className="border-t border-gray-100">
                <td className="px-4 py-2 text-gray-900">{tool.name}</td>
                <td className="px-4 py-2 font-mono text-xs text-gray-700">{tool.slug}</td>
                <td className="px-4 py-2 text-gray-600">{tool.version}</td>
                <td className="px-4 py-2 text-gray-600">{tool.kind}</td>
                <td className="px-4 py-2">
                  <StatusBadge status={tool.status} />
                </td>
                <td className="px-4 py-2">
                  <ScopeGate scopes={[AppScopes.ToolManage]}>
                    <button
                      type="button"
                      onClick={() => handleToggle(tool)}
                      disabled={busySlug === tool.slug}
                      className={`rounded-full px-2 py-0.5 text-xs font-medium disabled:opacity-50 ${
                        tool.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'
                      }`}
                    >
                      {tool.enabled ? 'Enabled' : 'Disabled'}
                    </button>
                  </ScopeGate>
                </td>
                <td className="px-4 py-2 text-right">
                  <ScopeGate scopes={[AppScopes.ToolManage]}>
                    <button
                      type="button"
                      onClick={() => handleDelete(tool)}
                      disabled={busySlug === tool.slug}
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
      {dialog}
    </div>
  )
}

function StatusBadge({ status }: { status: Tool['status'] }) {
  const colors: Record<Tool['status'], string> = {
    installed: 'bg-gray-100 text-gray-600',
    starting: 'bg-amber-100 text-amber-700',
    running: 'bg-green-100 text-green-700',
    stopped: 'bg-gray-100 text-gray-600',
    error: 'bg-red-100 text-red-700',
  }
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors[status]}`}>{status}</span>
}
