import { useEffect, useState } from 'react'
import { Dropdown } from '../../Components/Dropdown/Dropdown'
import { Switch } from '../../Shared/Switch/Switch'
import {
  fetchAutoDiscoverySettings,
  updateAutoDiscoverySettings,
  AUTO_DISCOVERY_INTERVAL_OPTIONS,
} from '../autoDiscovery'

// AutoDiscoveryPanel is the one global on/off + interval control for
// the auto-discovery scheduled-crawling feature (step 84) — a single
// setting covering every portal link that already has crawl
// instructions set, not a per-link schedule (see that step's own plan
// doc for why). Lives on PortalsPage.tsx, right alongside
// CrawlPlatformPicker, since both are page-wide crawling configuration
// rather than anything scoped to one portal or link. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
export function AutoDiscoveryPanel() {
  const [enabled, setEnabled] = useState(false)
  const [intervalMinutes, setIntervalMinutes] = useState(60)
  const [lastRunAt, setLastRunAt] = useState<number | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    fetchAutoDiscoverySettings()
      .then((s) => {
        setEnabled(s.enabled)
        setIntervalMinutes(s.intervalMinutes)
        setLastRunAt(s.lastRunAt)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to load auto-discovery settings.')
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  function save(nextEnabled: boolean, nextIntervalMinutes: number) {
    setError('')
    setSaving(true)
    // Optimistic — reverted below only if the request actually fails,
    // matching this tool's own established convention elsewhere
    // (JobsPage.tsx's own filter/match-status updates).
    setEnabled(nextEnabled)
    setIntervalMinutes(nextIntervalMinutes)
    updateAutoDiscoverySettings(nextEnabled, nextIntervalMinutes)
      .then((s) => {
        setEnabled(s.enabled)
        setIntervalMinutes(s.intervalMinutes)
        setLastRunAt(s.lastRunAt)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to update auto-discovery settings.')
        // Revert to what the server actually has, not just the
        // pre-optimistic values — a failed write should never leave
        // this panel showing a state the backend doesn't agree with.
        fetchAutoDiscoverySettings()
          .then((s) => {
            setEnabled(s.enabled)
            setIntervalMinutes(s.intervalMinutes)
            setLastRunAt(s.lastRunAt)
          })
          .catch(() => {
            // Best-effort — the error message above already explains
            // the failure; nothing further to show if even the
            // reload fails.
          })
      })
      .finally(() => {
        setSaving(false)
      })
  }

  if (!loaded) return null

  return (
    <div className="mb-5 flex flex-wrap items-end gap-3 rounded-md border border-gray-200 bg-gray-50 p-3">
      <Switch
        checked={enabled}
        disabled={saving}
        onChange={(nextEnabled) => {
          save(nextEnabled, intervalMinutes)
        }}
        label="Auto-discovery"
      />
      <div className="min-w-[160px]">
        <span className="mb-1 block text-xs font-medium text-gray-500">Crawl every</span>
        <Dropdown
          options={AUTO_DISCOVERY_INTERVAL_OPTIONS.map((o) => ({ value: String(o.value), label: o.label }))}
          value={String(intervalMinutes)}
          onChange={(value) => {
            save(enabled, Number(value))
          }}
        />
      </div>
      <span className="text-xs text-gray-500">
        {lastRunAt === null ? 'Never run yet.' : `Last run: ${new Date(lastRunAt * 1000).toLocaleString()}`}
      </span>
      {error && <span className="text-xs text-red-700">{error}</span>}
    </div>
  )
}
