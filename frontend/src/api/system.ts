import { apiGet } from './client'

// What the computer Cinqo runs on is doing (api/src/system): memory, and how much of it Cinqo itself holds. A value the
// computer could not read is 0 and the pressure "unknown".
export type MemoryPressure = 'normal' | 'warn' | 'critical' | 'unknown'

export interface SystemResources {
  memory: { totalBytes: number; availableBytes: number; swapUsedBytes: number; pressure: MemoryPressure }
  cinqo: { appBytes: number }
  sampledAt: string
}

export async function fetchSystemResources(signal?: AbortSignal): Promise<SystemResources> {
  const raw = await apiGet<{ message: SystemResources }>('/api/v1/system/resources', signal)
  return raw.message
}
