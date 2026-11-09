import { apiGet } from './client'

// What the computer Cinqo runs on is doing (api/src/system): memory, and how much of it Cinqo itself holds. A value the
// computer could not read is 0 and the pressure "unknown".
export type MemoryPressure = 'normal' | 'warn' | 'critical' | 'unknown'

export interface SystemResources {
  memory: { totalBytes: number; availableBytes: number; swapUsedBytes: number; pressure: MemoryPressure }
  // localAiBytes is the memory of the local model program, 0 while it does not run
  cinqo: { appBytes: number; localAiBytes: number }
  // load1 is the load average of the last minute (-1 when the system does not say); cores is the number of cores.
  cpu: { load1: number; cores: number }
  // The space of the volume the data folder is on; 0 when it cannot be read.
  disk: { freeBytes: number; totalBytes: number }
  // What the computer is; a fact that could not be read is an empty string.
  device: { model: string; modelId: string; chip: string; os: string; osVersion: string; arch: string; cores: number }
  sampledAt: string
}

export async function fetchSystemResources(signal?: AbortSignal): Promise<SystemResources> {
  const raw = await apiGet<{ message: SystemResources }>('/api/v1/system/resources', signal)
  return raw.message
}
