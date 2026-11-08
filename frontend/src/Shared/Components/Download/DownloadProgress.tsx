import { useId } from 'react'
import type { Download } from '../../../api/downloads'
import { amountText, barValueText, hasMeasure, isFinal, noticeText, paceText, phaseText } from './downloadText'
import { percent } from './format'

// The progress of one download: what is happening, a bar with the real share, how many bytes of how many, the speed and the
// time left, a clear failed state with the reason, and the buttons that make sense for the state. It only shows what it is
// given (see useDownload to follow a job); it never talks to the server.
const TONES = {
  light: {
    title: 'text-gray-900',
    text: 'text-gray-600',
    faint: 'text-gray-500',
    track: 'bg-gray-200',
    fail: 'text-red-700',
    notice: 'text-amber-700',
    button: 'text-gray-600 hover:text-gray-900',
    retry: 'text-teal-700 hover:text-teal-900',
  },
  dark: {
    title: 'text-white',
    text: 'text-white/80',
    faint: 'text-white/60',
    track: 'bg-white/20',
    fail: 'text-red-200',
    notice: 'text-amber-200',
    button: 'text-white/70 hover:text-white',
    retry: 'text-emerald-200 hover:text-white',
  },
} as const

export function DownloadProgress({
  download,
  error,
  onCancel,
  onRetry,
  busy = false,
  tone = 'light',
}: {
  download: Download | null
  // Set when the job cannot be followed any more (shown instead of the numbers).
  error?: string
  onCancel?: () => void
  // Offered for a failed or cancelled download (start it again; an interrupted one continues where it stopped).
  onRetry?: () => void
  // A cancel or a retry is on its way: the buttons wait.
  busy?: boolean
  // 'dark' is for a dark background (the Platforms slide).
  tone?: 'light' | 'dark'
}) {
  const t = TONES[tone]
  const labelId = useId()
  if (download === null) {
    return error ? (
      <p role="alert" className={`text-xs ${t.text}`}>
        {error}
      </p>
    ) : (
      <p className={`text-xs ${t.faint}`}>Looking up the download…</p>
    )
  }

  const final = isFinal(download.state)
  const failed = download.state === 'failed'
  const share = percent(download.bytesDone, download.bytesTotal)
  const measured = hasMeasure(download)
  const fill = download.state === 'done' ? 100 : measured ? share : 0
  const barColor = failed ? '#dc2626' : download.state === 'cancelled' ? '#9ca3af' : download.state === 'done' ? '#059669' : '#0d9488'
  const amount = amountText(download)
  const pace = paceText(download)
  const notice = noticeText(download)

  return (
    <div className="space-y-1.5" aria-labelledby={labelId}>
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p id={labelId} className={`text-xs font-medium ${t.title}`}>
          {download.label}
        </p>
        <p role="status" className={`text-xs ${failed ? `font-medium ${t.fail}` : t.text}`}>
          {phaseText(download)}
        </p>
      </div>

      <div
        role="progressbar"
        aria-labelledby={labelId}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={measured ? fill : undefined}
        aria-valuetext={measured ? barValueText(download) : phaseText(download)}
        className={`h-2 w-full overflow-hidden rounded-full ${t.track}`}
      >
        <div
          className={`h-full rounded-full transition-[width] duration-500 ease-out ${download.state === 'queued' || download.state === 'installing' ? 'animate-pulse' : ''}`}
          style={{ width: measured ? `${String(fill)}%` : '100%', background: barColor, opacity: measured || failed ? 1 : 0.35 }}
        />
      </div>

      <div className={`flex flex-wrap items-baseline justify-between gap-2 text-[11px] ${t.faint}`}>
        <span>{amount}</span>
        <span>{pace}</span>
      </div>

      {notice && <p className={`text-[11px] ${t.notice}`}>{notice}</p>}

      {failed && download.error && (
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
          {download.error}
        </p>
      )}
      {error && !final && <p className="text-[11px] text-amber-700">{error}</p>}

      {(!final && onCancel) || (final && download.state !== 'done' && onRetry) ? (
        <div className="flex gap-3">
          {!final && onCancel && (
            <button type="button" disabled={busy} onClick={onCancel} className={`text-xs font-medium disabled:opacity-50 ${t.button}`}>
              Cancel download
            </button>
          )}
          {final && download.state !== 'done' && onRetry && (
            <button type="button" disabled={busy} onClick={onRetry} className={`text-xs font-semibold disabled:opacity-50 ${t.retry}`}>
              {failed ? 'Try again' : 'Start again'}
            </button>
          )}
        </div>
      ) : null}
    </div>
  )
}
