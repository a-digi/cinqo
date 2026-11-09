import { formatBytes } from '../../../Shared/Components/Download/format'
import type {
  FileInstall,
  LocalAiEngine,
  LocalAiHardware,
  LocalAiModel,
  LocalAiResponse,
  LocalAiSettings,
  LocalAiStatus,
  LocalAiSuggestion,
  LocalAiTestResult,
} from './localAiApi'

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

// What a model can do with tools, in a line for its card.
export function toolsLine(m: Pick<LocalAiModel, 'tools'>): string {
  return m.tools
    ? 'Can use a few tools when a task names them (for example the CV import). Ask it to do something rather than asking a question: it may answer a question about your data without looking.'
    : 'Cannot use tools: it only answers.'
}

// The context the memory rule already allows for (the same constant as the server's): a larger context adds its own cache.
export const CONTEXT_BASELINE = 4096

// The memory a context size adds for this model, in bytes: its cache beyond the baseline (0 when the cache size is not stated).
export function contextCostBytes(model: Pick<LocalAiModel, 'kvBytesPerToken'> | undefined, tokens: number): number {
  if (!model || model.kvBytesPerToken <= 0 || !Number.isInteger(tokens) || tokens <= CONTEXT_BASELINE) return 0
  return model.kvBytesPerToken * (tokens - CONTEXT_BASELINE)
}

// What the model needs in free memory to start now, in bytes: its file plus a quarter for the runtime, plus the cache of a context beyond the
// baseline. The same rule as the server's (api/src/localai/hardware/fit.go) without its 3 GiB reserve for the system: what the system and the
// other programs already hold is not free any more, so the live figure of the resource bar already accounts for it (plan 222, step 5).
export function runNeedBytes(model: Pick<LocalAiModel, 'sizeBytes' | 'kvBytesPerToken'>, tokens: number): number {
  return Math.ceil((model.sizeBytes * 5) / 4) + contextCostBytes(model, tokens)
}

export interface MemoryShortage {
  needed: number
  free: number
}

// Whether starting the chosen model now would need more memory than is free right now. Only for a model that is on this computer and not
// already loaded (a loaded model's memory is already counted as used); null when nothing can be said (free memory unknown) or all is fine.
// This only warns: the person can start anyway.
export function memoryShortage(
  model: Pick<LocalAiModel, 'id' | 'sizeBytes' | 'kvBytesPerToken' | 'install'> | undefined,
  tokens: number,
  freeBytes: number | null,
  engine: Pick<LocalAiEngine, 'state' | 'modelId'>,
): MemoryShortage | null {
  if (!model || freeBytes === null || freeBytes <= 0 || model.install.state !== 'ready') return null
  if ((engine.state === 'ready' || engine.state === 'starting') && engine.modelId === model.id) return null
  const needed = runNeedBytes(model, tokens)
  return needed > freeBytes ? { needed, free: freeBytes } : null
}

export function shortageText(s: MemoryShortage, modelName: string): string {
  return `${modelName} needs about ${formatBytes(s.needed)} to start, and only ${formatBytes(s.free)} of memory is free right now. Close other programs first, or start anyway: the computer may slow down badly or the model program may be stopped.`
}

// Whether this computer has the memory for a context size with this model; true when that is not known (never held against it).
export function contextFits(model: Pick<LocalAiModel, 'largestContext'> | undefined, tokens: number): boolean {
  const largest = model?.largestContext
  if (largest === undefined || largest === null || largest === 0) return true
  return tokens <= largest
}

export type ContextAdviceKind = 'none' | 'ok' | 'low' | 'cannot'

export interface ContextAdvice {
  kind: ContextAdviceKind
  text: string
  // The size to offer with one click (only for 'low').
  size: number | null
}

// What to tell about the chosen context size for a model that can use tools: nothing for a model that cannot, a confirmation when
// the size is enough, a recommendation (with the size to take) when it is too small, and a warning when this computer cannot hold
// the recommended size at all.
export function contextAdvice(
  tokens: number,
  model: Pick<LocalAiModel, 'recommendedContext' | 'largestContext'> | undefined,
): ContextAdvice {
  const recommended = model?.recommendedContext
  if (recommended === undefined || recommended === null) return { kind: 'none', text: '', size: null }
  const shown = recommended.toLocaleString('en-US')
  if (tokens >= recommended)
    return { kind: 'ok', text: `Enough for tool tasks such as importing a CV (they need at least ${shown} tokens).`, size: null }
  if (!contextFits(model, recommended)) {
    const largest = (model?.largestContext ?? 0).toLocaleString('en-US')
    return {
      kind: 'cannot',
      text: `Tool tasks such as importing a CV need at least ${shown} tokens, but this computer has the memory for ${largest} at most with this model.`,
      size: null,
    }
  }
  return {
    kind: 'low',
    text: `${tokens.toLocaleString('en-US')} tokens is too small for tool tasks such as importing a CV: they need at least ${shown}.`,
    size: recommended,
  }
}

// One option of the context size list: the size and what it costs.
export function contextLabel(tokens: number, model: Pick<LocalAiModel, 'kvBytesPerToken' | 'largestContext'> | undefined): string {
  const base = `${tokens.toLocaleString('en-US')} tokens`
  const cost = contextCostBytes(model, tokens)
  const extra = cost > 0 ? ` (+${formatBytes(cost)} memory)` : ''
  return contextFits(model, tokens) ? base + extra : `${base}${extra}: needs more memory than this computer has`
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

// --- plan 205: switching on sets everything up in the background ---

export type SwitchVisual = 'on' | 'pending' | 'off'

// How the switch looks: ON only when the local AI is really ready; amber while it is switched on but being set up (or stuck); off otherwise.
export function switchVisual(saved: Pick<LocalAiSettings, 'enabled'>, status: Pick<LocalAiStatus, 'state'>): SwitchVisual {
  if (!saved.enabled || status.state === 'off') return 'off'
  return status.state === 'configured' ? 'on' : 'pending'
}

export type SetupAction = 'continue' | 'retry' | 'switch_off'

export interface SetupView {
  kind: 'none' | 'working' | 'ready' | 'failed' | 'interrupted'
  title: string
  detail: string
  actions: SetupAction[]
}

// What the page says under the switch about setting the local AI up: what is being downloaded, that it is ready, why it stopped, or that
// it was interrupted. Pure: the words and the actions are decided here, the component only shows them.
export function setupView(saved: Pick<LocalAiSettings, 'enabled'>, status: LocalAiStatus): SetupView {
  if (!saved.enabled || status.state === 'off') return { kind: 'none', title: '', detail: '', actions: [] }
  switch (status.state) {
    case 'preparing': {
      const p = status.preparing
      const what =
        p === null
          ? 'Downloading what the local AI needs.'
          : p.step === 'program'
            ? `Downloading the program that runs the models (${p.name}, ${formatBytes(p.sizeBytes)}). It is installed when the download is done.`
            : `Downloading the model (${p.name}, ${formatBytes(p.sizeBytes)}).`
      return { kind: 'working', title: 'Setting up the local AI…', detail: what, actions: [] }
    }
    case 'configured':
      return { kind: 'ready', title: 'Ready', detail: 'Conversations can use the local model.', actions: [] }
    case 'failed': {
      const f = status.lastFailure
      const message = f === null || f.message === '' ? 'Something went wrong.' : f.message
      const title =
        f?.what === 'program'
          ? 'The program that runs the models could not be set up'
          : f?.what === 'model'
            ? 'The model could not be downloaded'
            : 'The local AI cannot run here'
      return { kind: 'failed', title, detail: message, actions: f?.what === 'system' ? ['switch_off'] : ['retry', 'switch_off'] }
    }
    case 'interrupted':
      return {
        kind: 'interrupted',
        title: 'Setting up was interrupted',
        detail:
          'Cinqo has not finished downloading what the local AI needs (the app was restarted, or a file was removed). Continue to download the rest.',
        actions: ['continue', 'switch_off'],
      }
  }
}

// ---- models that tools suggested (plan 220) ----

// "Career", "Career and Browser", "Career, Browser and PDF Tools": names in a sentence. Empty names are left out, none at all reads "a tool".
export function joinTools(names: string[]): string {
  const list = names.map((n) => n.trim()).filter((n) => n !== '')
  if (list.length === 0) return 'a tool'
  if (list.length === 1) return list[0]
  return `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`
}

// "Added by Career and Browser": every tool that suggested a model that was added (the first one when the core lists only one). A model Cinqo offers
// on its own is "Added by Cinqo".
export function addedByText(m: Pick<LocalAiModel, 'origin' | 'addedBy' | 'suggestedBy'>): string {
  if (m.origin !== 'added') return 'Added by Cinqo'
  const all = m.suggestedBy ?? []
  return `Added by ${joinTools(all.length > 0 ? all : [m.addedBy ?? ''])}`
}

// Where an added model is from, in a line for its card.
export function addedSourceLine(m: Pick<LocalAiModel, 'origin' | 'repo' | 'publisher'>): string {
  if (m.origin !== 'added') return ''
  return `Hugging Face ${m.repo ?? ''}, published by ${m.publisher ?? 'unknown'}. Cinqo has not tested this model: it only answers, it gets no tools.`
}

// Whether the Remove button is offered: only an added model, never the one chosen as the default, never while it is being downloaded.
export function canRemoveModel(m: Pick<LocalAiModel, 'id' | 'origin' | 'install'>, savedModelId: string): boolean {
  return m.origin === 'added' && m.id !== savedModelId && m.install.state !== 'downloading'
}

export function suggestionFacts(s: LocalAiSuggestion): string {
  return `${formatBytes(s.sizeBytes)} download · needs about ${formatBytes(s.minRamBytes)} memory · ${s.quantization} · License ${s.license}`
}

// What each tool said about it, always as the tool's own words: "Career says: ...". One line per tool, first suggester first. A core that lists
// only the first tool still gives its line.
export function suggestionWhyLines(s: Pick<LocalAiSuggestion, 'suggestedBy' | 'why' | 'suggesters'>): string[] {
  const all = s.suggesters !== undefined && s.suggesters.length > 0 ? s.suggesters : [{ tool: s.suggestedBy, why: s.why, at: '' }]
  return all.map((x) => {
    const by = x.tool.trim() === '' ? 'A tool' : x.tool.trim()
    return x.why.trim() === '' ? `${by} suggested this model.` : `${by} says: ${x.why.trim()}`
  })
}

// "Suggested by Career and Browser": who wants the model, in one line for the head of the card.
export function suggestedByText(s: Pick<LocalAiSuggestion, 'suggestedBy' | 'suggesters'>): string {
  const all = s.suggesters !== undefined && s.suggesters.length > 0 ? s.suggesters.map((x) => x.tool) : [s.suggestedBy]
  return `Suggested by ${joinTools(all)}`
}

// The question asked before anything is downloaded: where it is from, who made it, how big it is.
export function suggestionConfirmMessage(s: Pick<LocalAiSuggestion, 'name' | 'repo' | 'publisher' | 'sizeBytes' | 'shipped'>): string {
  if (s.shipped === true) {
    return `Download ${s.name} (${formatBytes(s.sizeBytes)})? It is one of the models that ship with Cinqo, checked by Cinqo, and it will run on your computer.`
  }
  return `Download ${s.name} (${formatBytes(s.sizeBytes)}) from Hugging Face (${s.repo}, published by ${s.publisher})? Cinqo will run this model on your computer. Cinqo checks the file, but has not tested the model.`
}

export function removeModelMessage(m: Pick<LocalAiModel, 'name'>): string {
  return `Remove ${m.name}? Its downloaded file is deleted. A tool can suggest it again later.`
}
