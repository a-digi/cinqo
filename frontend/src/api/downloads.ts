import { apiDelete, apiGet } from './client'

// The generic download manager's API (api/src/download): watch a download and cancel it. Starting one is the business of
// the feature that downloads, behind its own endpoint.

export type DownloadState = 'queued' | 'downloading' | 'verifying' | 'installing' | 'done' | 'failed' | 'cancelled'

// bytesDone and bytesTotal count the bytes received while downloading and the bytes checked while verifying.
// secondsLeft is null while it cannot be known. stalled is true while no byte has arrived for a while.
export interface Download {
  id: string
  label: string
  state: DownloadState
  bytesDone: number
  bytesTotal: number
  bytesPerSecond: number
  secondsLeft: number | null
  stalled: boolean
  // Not empty while the connection was lost and the manager waits to reconnect at the byte where it stopped.
  notice: string
  error: string
  startedAt: string
  finishedAt: string
}

export async function fetchDownload(id: string, signal?: AbortSignal): Promise<Download> {
  const raw = await apiGet<{ message: Download }>(`/api/v1/downloads/${encodeURIComponent(id)}`, signal)
  return raw.message
}

export async function cancelDownload(id: string): Promise<void> {
  await apiDelete(`/api/v1/downloads/${encodeURIComponent(id)}`)
}
