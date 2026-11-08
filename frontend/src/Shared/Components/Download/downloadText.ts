import type { Download, DownloadState } from '../../../api/downloads'
import { formatBytes, formatSpeed, formatTimeLeft, percent } from './format'

// What the progress display says for each state. Pure (no React), so the wording can be script-checked.

// A finished job (in any way) does not change any more: the display stops asking.
export function isFinal(state: DownloadState): boolean {
  return state === 'done' || state === 'failed' || state === 'cancelled'
}

// The line above the bar: what is happening right now.
export function phaseText(d: Download): string {
  switch (d.state) {
    case 'queued':
      return 'Waiting to start…'
    case 'downloading':
      if (d.notice !== '') return 'Reconnecting…'
      return d.stalled ? 'No data has arrived for a while…' : 'Downloading…'
    case 'verifying':
      return 'Checking the file…'
    case 'installing':
      return 'Setting it up…'
    case 'done':
      return 'Downloaded'
    case 'cancelled':
      return 'Cancelled'
    case 'failed':
      return 'The download failed'
  }
}

// "1.2 GiB of 2.3 GiB · 52%": how far it is (while downloading or checking).
export function amountText(d: Download): string {
  if (d.state === 'queued') return ''
  if (d.state === 'installing') return ''
  const total = formatBytes(d.bytesTotal)
  if (total === 'unknown') return ''
  return `${d.bytesDone > 0 ? formatBytes(d.bytesDone) : '0 B'} of ${total} · ${String(percent(d.bytesDone, d.bytesTotal))}%`
}

// "12.4 MiB/s · about 3 minutes left": only while downloading, and only what is known; "calculating…" while the speed is not
// known yet.
export function paceText(d: Download): string {
  if (d.state !== 'downloading' || d.stalled || d.notice !== '') return ''
  const speed = formatSpeed(d.bytesPerSecond)
  const left = formatTimeLeft(d.secondsLeft)
  if (speed === '') return 'calculating the speed…'
  return left === '' ? speed : `${speed} · ${left} left`
}

// The line about a lost connection while the download waits to reconnect ("Trying again in 4 s…"), else nothing.
export function noticeText(d: Download): string {
  return d.state === 'downloading' ? d.notice : ''
}

// The value for a screen reader's progress bar: the share and what the bar is about.
export function barValueText(d: Download): string {
  return `${phaseText(d).replace('…', '')}: ${String(percent(d.bytesDone, d.bytesTotal))}%`
}

// Whether the bar shows a real share (false: queued or installing, where there is nothing to measure). A failed or cancelled
// download shows how far it got, so a failure at 56% does not look like 100%.
export function hasMeasure(d: Download): boolean {
  return d.state !== 'queued' && d.state !== 'installing'
}
