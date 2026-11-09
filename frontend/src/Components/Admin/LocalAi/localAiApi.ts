import { apiDelete, apiGet, apiPost, apiPut } from '../../../api/client'

// The local AI module's own API client (api/src/localai). Nothing outside Components/Admin/LocalAi imports it.
// plan/ai/small-llm-local/step-181-local-ai-small-models.md

// The local AI is managed by Cinqo only: modelId is the id of a model of the catalog, never free text.
export interface LocalAiSettings {
  enabled: boolean
  modelId: string
  contextTokens: number
  idleMinutes: number
}

// 'off': switched off or no model chosen. 'preparing': switched on, the program and/or the model are being downloaded now.
// 'configured': switched on and ready (fully on). 'failed': switched on, not ready, the last attempt failed (or this computer cannot
// run it). 'interrupted': switched on, not ready, nothing is downloading (the app was restarted, or a file was removed).
export type LocalAiState = 'off' | 'preparing' | 'configured' | 'failed' | 'interrupted'

// The download in progress while the state is 'preparing': the program first, then the model. downloadId is followed with the
// download API (it also says downloading or installing).
export interface PreparingStep {
  step: 'program' | 'model'
  downloadId: string
  name: string
  sizeBytes: number
}

// Why the last download did not finish (state 'failed'): what is 'program', 'model' or 'system' (nothing a download fixes).
export interface LocalAiFailure {
  what: 'program' | 'model' | 'system'
  message: string
  at: string
}

// canEnable says whether the local AI can be switched on at all right now (nothing a download fixes is in the way); reason says why not
// when it cannot.
export interface LocalAiStatus {
  state: LocalAiState
  canEnable: boolean
  reason: string
  preparing: PreparingStep | null
  lastFailure: LocalAiFailure | null
}

// What this machine can run. ramBytes is 0 when it could not be read; supported says whether the managed local AI is meant
// to run on this system at all.
export interface LocalAiHardware {
  os: string
  arch: string
  ramBytes: number
  cpus: number
  supported: boolean
}

// Where the files of a model, or of the runtime, are: nothing yet, an interrupted download that can continue, a download
// that is running (downloadId is the one to follow with the download API), or ready.
export type InstallState = 'not_downloaded' | 'partial' | 'downloading' | 'ready'

export interface FileInstall {
  state: InstallState
  partialBytes: number
  downloadId: string
}

// The program that runs the models, for this system. supported is false on a system that has none yet.
export interface LocalAiRuntime {
  supported: boolean
  name: string
  sizeBytes: number
  install: FileInstall
}

// The model program (llama-server, started by Cinqo): stopped (it starts when a conversation needs it), starting, ready or failed.
export type EngineState = 'stopped' | 'starting' | 'ready' | 'failed'

export interface LocalAiEngine {
  state: EngineState
  error: string
  modelId: string
}

// Whether the local AI can be switched on with a model right now, and if not, why not.
export interface EnableState {
  ok: boolean
  reason: string
}

// One model Cinqo knows how to download. fits is null when the memory of this machine is unknown.
export interface LocalAiModel {
  id: string
  name: string
  quantization: string
  sizeBytes: number
  minRamBytes: number
  contextTokens: number
  // The model was measured to call tools reliably (a conversation may then be given a list of tools).
  tools: boolean
  // What the model's cache holds per token of context (0: not stated), so a context size can show what it costs in memory.
  kvBytesPerToken: number
  // The largest context size offered that this computer has the memory for with this model; null when that is not known.
  largestContext: number | null
  // The context size recommended for tool tasks (a CV import, for example) with this model; null for a model that cannot use tools.
  recommendedContext: number | null
  license: string
  source: string
  note: string
  checkedOn: string
  fits: boolean | null
  install: FileInstall
  enable: EnableState
  // 'catalog': shipped with Cinqo. 'added': a model a person added after a tool suggested it (plan 220); addedBy is who suggested it (the tool's own word),
  // repo and publisher say where it is from and why is the reason given.
  origin: 'catalog' | 'added'
  addedBy?: string
  repo?: string
  publisher?: string
  why?: string
}

// A model a tool suggested that nobody confirmed yet: nothing of it is downloaded. The address and the hash are not sent. fits is null when the memory
// of this machine is unknown.
export interface LocalAiSuggestion {
  id: string
  name: string
  repo: string
  file: string
  commit: string
  publisher: string
  sizeBytes: number
  minRamBytes: number
  quantization: string
  license: string
  contextTokens: number
  why: string
  suggestedBy: string
  status: string
  createdAt: string
  fits: boolean | null
  // A suggestion of a model that ships with Cinqo (checked and measured by Cinqo): confirming it only downloads it.
  shipped?: boolean
}

export interface LocalAiResponse {
  settings: LocalAiSettings
  status: LocalAiStatus
  hardware: LocalAiHardware
  catalog: LocalAiModel[]
  catalogCheckedOn: string
  runtime: LocalAiRuntime
  engine: LocalAiEngine
  // The suggestions that wait for a decision (empty from a core that predates them).
  suggestions?: LocalAiSuggestion[]
}

export async function fetchLocalAi(): Promise<LocalAiResponse> {
  const raw = await apiGet<{ message: LocalAiResponse }>('/api/v1/local-ai')
  return raw.message
}

export async function updateLocalAiSettings(settings: LocalAiSettings): Promise<LocalAiResponse> {
  const raw = await apiPut<{ message: LocalAiResponse }>('/api/v1/local-ai/settings', settings)
  return raw.message
}

// Starts downloading the runtime and the model (the model of the settings when no id is given). The answer is the status,
// which now carries the ids of the running downloads.
export async function startInstall(modelId?: string): Promise<LocalAiResponse> {
  const raw = await apiPost<{ message: LocalAiResponse }>('/api/v1/local-ai/install', modelId === undefined ? {} : { modelId })
  return raw.message
}

// Deletes everything that was downloaded (refused while the local AI is on or a download is running).
export async function removeDownloads(): Promise<void> {
  await apiDelete('/api/v1/local-ai/install')
}

// The outcome of the test: one tiny real conversation with the model, timed. A failure of the model is part of the result
// (ok false with the reason), not an error.
export interface LocalAiTestResult {
  ok: boolean
  error: string
  startSeconds: number
  seconds: number
  promptTokens: number
  completionTokens: number
  tokensPerSecond: number
  reply: string
}

// Runs the test on the model (the model of the settings when no id is given). It can take a while: the model program is
// started first when it is not running.
export async function testModel(modelId?: string): Promise<LocalAiTestResult> {
  const raw = await apiPost<{ message: LocalAiTestResult }>('/api/v1/local-ai/test', modelId === undefined ? {} : { modelId })
  return raw.message
}

// Confirms a suggested model: it becomes part of the list of models and its download starts. The answer is the page's own.
export async function confirmSuggestion(id: string): Promise<LocalAiResponse> {
  const raw = await apiPost<{ message: LocalAiResponse }>(`/api/v1/local-ai/models/${encodeURIComponent(id)}/install`, {})
  return raw.message
}

// Dismisses a suggestion: it is kept as dismissed, so a tool does not offer the same file again.
export async function dismissSuggestion(id: string): Promise<void> {
  await apiPost(`/api/v1/local-ai/suggestions/${encodeURIComponent(id)}/dismiss`, {})
}

// Removes one added model: its file and its entry. The answer is the page's own.
export async function removeAddedModel(id: string): Promise<LocalAiResponse> {
  const raw = await apiDelete<{ message: LocalAiResponse }>(`/api/v1/local-ai/models/${encodeURIComponent(id)}`)
  return raw.message
}
