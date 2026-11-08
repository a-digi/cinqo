import { useEffect, useRef } from 'react'
import { DownloadProgress } from '../../../Shared/Components/Download/DownloadProgress'
import { isFinal } from '../../../Shared/Components/Download/downloadText'
import { useDownload } from '../../../Shared/Components/Download/useDownload'
import type { Download } from '../../../api/downloads'
import type { FileInstall } from './localAiApi'
import { downloadButtonText } from './localAiFormat'

// Follows one running download and tells the page when it is over (or when the server no longer knows it), so the page can
// read the new state of the files.
export function ActiveDownload({
  id,
  busy,
  onCancel,
  onFinished,
}: {
  id: string
  busy: boolean
  onCancel: () => void
  onFinished: (d: Download | null) => void
}) {
  const { download, error } = useDownload(id)
  const reported = useRef('')
  useEffect(() => {
    if (download && isFinal(download.state) && reported.current !== download.id) {
      reported.current = download.id
      onFinished(download)
    } else if (!download && error !== '' && reported.current !== id) {
      reported.current = id
      onFinished(null)
    }
  }, [download, error, id, onFinished])
  return <DownloadProgress download={download} error={error} onCancel={onCancel} busy={busy} />
}

// What the files of one thing (a model, or the runtime) look like right now, and what can be done about it: a button to
// download, the live progress of a running download, a failed one with its reason and a retry, or "downloaded". `failure` is
// the last download of it that failed, kept by the page because the server forgets finished jobs.
export function InstallRow({
  install,
  sizeBytes,
  failure,
  supported,
  canManage,
  busy,
  onStart,
  onCancel,
  onFinished,
}: {
  install: FileInstall
  sizeBytes: number
  failure: Download | null
  supported: boolean
  canManage: boolean
  busy: boolean
  onStart: () => void
  onCancel: (downloadId: string) => void
  onFinished: (d: Download | null) => void
}) {
  if (install.state === 'downloading' && install.downloadId !== '') {
    return (
      <ActiveDownload
        id={install.downloadId}
        busy={busy}
        onCancel={() => {
          onCancel(install.downloadId)
        }}
        onFinished={onFinished}
      />
    )
  }
  if (install.state === 'ready') {
    return <p className="text-xs font-medium text-teal-700">✓ Downloaded</p>
  }
  if (!supported) {
    return <p className="text-xs text-gray-500">Not available for this system yet.</p>
  }
  return (
    <div className="space-y-2">
      {failure && <DownloadProgress download={failure} onRetry={canManage ? onStart : undefined} busy={busy} />}
      {canManage && !failure && (
        <button
          type="button"
          disabled={busy}
          onClick={onStart}
          className="rounded-full border border-teal-600 px-3 py-1 text-xs font-semibold text-teal-700 transition hover:bg-teal-50 disabled:opacity-50"
        >
          {downloadButtonText(install, sizeBytes)}
        </button>
      )}
    </div>
  )
}
