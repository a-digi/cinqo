import { useRef, useState } from 'react'
import { installTool } from '../../../api/tools'
import { ApiError } from '../../../api/client'
import { reloadAfterToolChange } from '../../../config/tools/loadTools'

// Hidden <input type="file"> + a visible button — same shape as
// coco-mda's real PluginInstallButton. See
// plan/ai/tools/step-08-frontend-admin-ui.md.
export function ToolInstallButton() {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same file next time
    if (!file) return

    setBusy(true)
    setError(null)
    try {
      await installTool(file)
      reloadAfterToolChange()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to install tool.')
      setBusy(false)
    }
  }

  return (
    <div>
      <input
        ref={inputRef}
        type="file"
        accept=".zip,application/zip"
        className="hidden"
        onChange={(e) => {
          void handleChange(e)
        }}
        disabled={busy}
      />
      <button
        type="button"
        onClick={() => inputRef.current?.click()}
        disabled={busy}
        className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
      >
        {busy ? 'Installing…' : 'Install tool'}
      </button>
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
    </div>
  )
}
