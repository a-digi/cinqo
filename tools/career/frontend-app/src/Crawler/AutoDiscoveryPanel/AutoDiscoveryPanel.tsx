import { useEffect, useState } from 'react'
import { Dropdown } from '../../Components/Dropdown/Dropdown'
import { BoltIcon } from '../../Shared/Icons/icons'
import { Switch } from '../../Shared/Switch/Switch'
import { fetchAutoDiscoverySettings, updateAutoDiscoverySettings, AUTO_DISCOVERY_INTERVAL_OPTIONS } from '../autoDiscovery'

// AutoDiscoveryPanel is the one global on/off + interval control for
// the auto-discovery scheduled-crawling feature (step 84) — a single
// setting covering every portal link that already has crawl
// instructions set, not a per-link schedule (see that step's own plan
// doc for why). Lives on PortalsPage.tsx as page-wide crawling
// configuration rather than anything scoped to one portal or link (the AI
// platform and model it uses are the tool-wide Settings choice). It sits
// at the top of the page's left column, above the Overview (step 104), so
// it is separated from what follows by a line under it, and keeps its
// place with an empty block while its settings load, so the Overview does
// not jump down when they arrive. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md and
// plan/ai/tools/career/step-104-auto-discovery-above-the-overview.md.
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

  // The loaded panel's own height (156px measured live: content, padding and its line), plus
  // the same space under it, so the Overview does not move when the settings arrive.
  if (!loaded) return <div className="mb-6 h-[9.75rem]" aria-hidden="true" />

  return (
    <div className="mb-6 border-b border-gray-200 pb-5">
      <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
        <span style={{ color: '#7c3aed' }}>
          <BoltIcon className="h-4 w-4" />
        </span>
        Auto-discovery
      </div>
      <div className="flex flex-col gap-3">
        <Switch
          checked={enabled}
          disabled={saving}
          onChange={(nextEnabled) => {
            save(nextEnabled, intervalMinutes)
          }}
          label={enabled ? 'On: crawls on a schedule' : 'Off'}
        />
        <div className="[&>div]:block [&>div>button]:w-full [&>div>button]:min-w-0">
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
    </div>
  )
}
