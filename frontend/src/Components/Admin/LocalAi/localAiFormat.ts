import { formatBytes } from '../../../Shared/Components/Download/format'
import type { FileInstall, LocalAiEngine, LocalAiHardware, LocalAiModel, LocalAiResponse, LocalAiTestResult } from './localAiApi'

// How the page and the slide put sizes, the machine and the fit into words. Pure (no React), so the rules can be
// script-checked.

const OS_NAMES: Record<string, string> = { darwin: 'macOS', linux: 'Linux', windows: 'Windows' }

export function osName(os: string): string {
  return OS_NAMES[os] ?? (os === '' ? 'Unknown system' : os)
}

// "Apple Silicon" is only said for arm64 on macOS; every other processor family is shown as the system reports it.
export function archName(os: string, arch: string): string {
  if (os === 'darwin' && arch === 'arm64') return 'Apple Silicon'
  return arch === '' ? 'unknown processor' : arch
}

export function hardwareSummary(h: LocalAiHardware): string {
  const parts = [
    osName(h.os),
    archName(h.os, h.arch),
    `${formatBytes(h.ramBytes)} memory`,
    `${String(h.cpus)} ${h.cpus === 1 ? 'core' : 'cores'}`,
  ]
  return parts.join(' · ')
}

export type FitKind = 'fits' | 'tight' | 'unknown'

// "fits" says the machine has the memory by the app's rule of thumb, "tight" that it has not, "unknown" that the memory
// could not be read (never a guess either way).
export function fitKind(fits: boolean | null): FitKind {
  if (fits === null) return 'unknown'
  return fits ? 'fits' : 'tight'
}

export function fitLabel(fits: boolean | null): string {
  switch (fitKind(fits)) {
    case 'fits':
      return 'Fits this computer'
    case 'tight':
      return 'Needs more memory'
    case 'unknown':
      return 'Memory unknown'
  }
}

// One line about a model: its file, the memory it needs and how much it can read at once.
export function modelFacts(m: LocalAiModel): string {
  return `${formatBytes(m.sizeBytes)} download · needs about ${formatBytes(m.minRamBytes)} memory · reads up to ${m.contextTokens.toLocaleString('en-US')} tokens`
}

// What the page and the slide say about running models on this machine.
export function supportText(h: LocalAiHardware): string {
  return h.supported
    ? 'Cinqo can run a model on this system once the local AI is ready.'
    : 'Not supported yet: the local AI is being built and tested on macOS (Apple Silicon) and Linux (amd64) first, and Windows is planned.'
}

// The text of the button that starts, or continues, a download.
export function downloadButtonText(install: FileInstall, sizeBytes: number): string {
  if (install.state === 'partial' && install.partialBytes > 0) {
    return `Continue the download (${formatBytes(install.partialBytes)} of ${formatBytes(sizeBytes)} are already here)`
  }
  return `Download (${formatBytes(sizeBytes)})`
}

// Whether anything that was downloaded is on this computer, in whole or in part (so there is something to remove).
export function hasDownloadedFiles(data: LocalAiResponse): boolean {
  const there = (i: FileInstall) => i.state === 'ready' || i.state === 'partial'
  return there(data.runtime.install) || data.catalog.some((m) => there(m.install))
}

// Whether a download is running now.
export function isDownloading(data: LocalAiResponse): boolean {
  return data.runtime.install.state === 'downloading' || data.catalog.some((m) => m.install.state === 'downloading')
}

// The ids of the running downloads (the runtime's first, then the models').
export function runningDownloadIds(data: LocalAiResponse): string[] {
  const ids: string[] = []
  if (data.runtime.install.state === 'downloading' && data.runtime.install.downloadId !== '') ids.push(data.runtime.install.downloadId)
  for (const m of data.catalog) if (m.install.state === 'downloading' && m.install.downloadId !== '') ids.push(m.install.downloadId)
  return ids
}

// What the model program is doing, in a line.
export function engineLine(e: LocalAiEngine): string {
  switch (e.state) {
    case 'stopped':
      return 'Not running. It starts when a conversation needs it.'
    case 'starting':
      return 'Starting: the model is loading…'
    case 'ready':
      return 'Running and ready.'
    case 'failed':
      return e.error === '' ? 'It stopped unexpectedly.' : `It failed: ${e.error}`
  }
}

// A duration for a person: "0.4 s", "2.5 s", "75 s", "3 min 5 s".
export function formatSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return ''
  if (seconds < 10) return `${seconds.toFixed(1)} s`
  const whole = Math.round(seconds)
  if (whole < 120) return `${String(whole)} s`
  const minutes = Math.floor(whole / 60)
  const rest = whole - minutes * 60
  return rest === 0 ? `${String(minutes)} min` : `${String(minutes)} min ${String(rest)} s`
}

// The result of the test in one or two sentences: how long the answer took and how fast it was, and, when the model had to
// be loaded first, how long that took.
export function testSummary(r: LocalAiTestResult): string {
  if (!r.ok) return `The test failed: ${r.error === '' ? 'the model did not answer.' : r.error}`
  const parts = [`It answered "${r.reply}" in ${formatSeconds(r.seconds)}`]
  if (r.tokensPerSecond > 0)
    parts[0] += ` (${r.tokensPerSecond >= 10 ? String(Math.round(r.tokensPerSecond)) : r.tokensPerSecond.toFixed(1)} tokens per second)`
  parts[0] += '.'
  if (r.startSeconds >= 0.5) parts.push(`Loading the model took ${formatSeconds(r.startSeconds)}.`)
  else parts.push('The model was already running.')
  return parts.join(' ')
}

// What the idle setting means.
export function idleHelp(minutes: number): string {
  if (!Number.isInteger(minutes) || minutes < 0) return ''
  if (minutes === 0) return 'The model stays loaded until Cinqo stops.'
  return `The model is unloaded after ${String(minutes)} ${minutes === 1 ? 'minute' : 'minutes'} without a conversation, and loaded again when one needs it.`
}

// True when nothing can be switched on yet and it is not on: the local AI still needs a download (or more memory).
export function needsSetup(data: LocalAiResponse): boolean {
  return !data.settings.enabled && !data.catalog.some((m) => m.enable.ok)
}
