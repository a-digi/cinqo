import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import type { SystemResources } from '../api/system'

// How tight the memory is (plan 222). Judged by what is still available, not by what is "used": the system keeps caches in "used".
// unknown = the computer did not say; it is shown grey and never as an alarm.
export type ResourceLevel = 'unknown' | 'ok' | 'orange' | 'red'

const GB = 1024 ** 3
const ORANGE_AVAILABLE = 0.25
const RED_AVAILABLE = 0.1
const ORANGE_SWAP = 1 * GB
const RED_SWAP = 3 * GB

export function resourceLevel(r: SystemResources): ResourceLevel {
  const { totalBytes, availableBytes, swapUsedBytes, pressure } = r.memory
  if (totalBytes <= 0 || (availableBytes <= 0 && pressure === 'unknown')) return 'unknown'
  const free = availableBytes / totalBytes
  if (pressure === 'critical' || free < RED_AVAILABLE || swapUsedBytes > RED_SWAP) return 'red'
  if (pressure === 'warn' || free < ORANGE_AVAILABLE || swapUsedBytes > ORANGE_SWAP) return 'orange'
  return 'ok'
}

// The words of the pill: the numbers while all is well, the reason when it is not.
export function pillText(r: SystemResources, level: ResourceLevel): string {
  const { totalBytes, availableBytes } = r.memory
  if (level === 'unknown') return 'Memory: unknown'
  if (level === 'ok') return `RAM ${formatBytes(totalBytes - availableBytes)} / ${formatBytes(totalBytes)} used`
  return `Memory is ${level === 'red' ? 'very tight' : 'tight'}: ${formatBytes(availableBytes)} free of ${formatBytes(totalBytes)}`
}

// The sentence for the title attribute and screen readers.
export function pillTitle(r: SystemResources): string {
  const { totalBytes, availableBytes, swapUsedBytes, pressure } = r.memory
  const parts = [
    `Total ${formatBytes(totalBytes)}`,
    `available ${formatBytes(availableBytes)}`,
    `swap in use ${formatBytes(swapUsedBytes)}`,
  ]
  if (pressure !== 'unknown') parts.push(`system pressure ${pressure}`)
  return parts.join(', ')
}

// What the computer is, in one line: "MacBook Air · Apple M1 · macOS 15.1 · 8 GB". A fact that is not known is left out; empty when none is.
export function deviceText(r: SystemResources): string {
  const d = r.device
  const system = [d.os, d.osVersion].filter((x) => x !== '').join(' ')
  const parts = [d.model !== '' ? d.model : d.modelId, d.chip, system, r.memory.totalBytes > 0 ? formatBytes(r.memory.totalBytes) : '']
  return parts.filter((x) => x !== '').join(' · ')
}
