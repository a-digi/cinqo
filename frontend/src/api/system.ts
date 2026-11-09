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
  // The local agents that run now (today the local model program); empty when none does.
  agents: SystemAgent[]
  // What the computer is; a fact that could not be read is an empty string.
  device: { model: string; modelId: string; chip: string; os: string; osVersion: string; arch: string; cores: number }
  sampledAt: string
}

export type AgentState = 'starting' | 'working' | 'idle' | 'failed'

// working: a request is under way; idle: loaded and waiting (unloadsInSeconds counts down to its unloading, 0 = never); failed: it stopped on its own.
export interface SystemAgent {
  kind: string
  name: string
  state: AgentState
  memoryBytes: number
  unloadsInSeconds: number
}

export async function fetchSystemResources(signal?: AbortSignal): Promise<SystemResources> {
  const raw = await apiGet<{ message: SystemResources }>('/api/v1/system/resources', signal)
  return raw.message
}

// The applications that use the most memory (api/src/system, plan 226): one row per application, its processes summed.
export interface MemoryTopApp {
  name: string
  pid: number // the application's biggest process
  processes: number
  rssBytes: number
  cinqo: boolean // Cinqo's own program, its tools and the model program, merged into one row
}

export interface MemoryTop {
  sampledAt: string
  totalBytes: number
  apps: MemoryTopApp[]
  note?: string // why the list is empty, when the system cannot be read
}

export async function fetchMemoryTop(signal?: AbortSignal): Promise<MemoryTop> {
  const raw = await apiGet<{ message: MemoryTop }>('/api/v1/system/memory-top', signal)
  return raw.message
}
