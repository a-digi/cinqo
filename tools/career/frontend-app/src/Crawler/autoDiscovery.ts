// autoDiscovery.ts — the auto-discovery scheduled-crawling feature's
// own plain proxy calls (step 84): one global on/off + interval
// setting, read/written against Career's own backend
// (tools/career/backend/autodiscovery/settings.go). No polling here —
// unlike crawlNow.ts's own fetchActiveCrawlRun(s), this setting doesn't
// change on its own from outside this page, so there's nothing to
// watch; the manager that actually acts on it runs entirely
// server-side. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.

const AUTO_DISCOVERY_URL = '/api/v1/tools/career/proxy/portal-links/auto-discovery'

// intervalOptions is the fixed, exhaustive set of choices this feature
// ever offers — mirrors the backend's own allowedIntervalMinutes
// allowlist exactly (settings.go); the frontend never sends a value
// outside this list.
export const AUTO_DISCOVERY_INTERVAL_OPTIONS: { value: number; label: string }[] = [
  { value: 15, label: '15 minutes' },
  { value: 30, label: '30 minutes' },
  { value: 60, label: '1 hour' },
  { value: 180, label: '3 hours' },
  { value: 360, label: '6 hours' },
  { value: 720, label: '12 hours' },
  { value: 1440, label: '24 hours' },
]

export interface AutoDiscoverySettings {
  enabled: boolean
  intervalMinutes: number
  // lastRunAt is unix seconds (the backend's own storage shape,
  // step 84's own explicit "store as unix" requirement) — null means
  // auto-discovery has never actually run yet.
  lastRunAt: number | null
}

export async function fetchAutoDiscoverySettings(): Promise<AutoDiscoverySettings> {
  const res = await fetch(AUTO_DISCOVERY_URL, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load auto-discovery settings (${res.status})`)
  return res.json() as Promise<AutoDiscoverySettings>
}

export async function updateAutoDiscoverySettings(enabled: boolean, intervalMinutes: number): Promise<AutoDiscoverySettings> {
  const res = await fetch(AUTO_DISCOVERY_URL, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ enabled, intervalMinutes }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `failed to update auto-discovery settings (${res.status})`)
  }
  return res.json() as Promise<AutoDiscoverySettings>
}
