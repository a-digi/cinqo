package db

// AllowedAutoDiscoveryIntervalMinutes is the fixed set of interval
// choices the auto-discovery feature ever offers — 15m/30m/1h/3h/6h/
// 12h/24h — shared here (rather than declared once per consumer)
// because autodiscovery.UpdateSettings (the global interval) and
// portal.UpdatePortalLinkAutoDiscoveryInterval (a link's own override,
// step 85) both need the EXACT same allowlist and are two packages
// within this SAME module — same "share it once, in whatever package
// both already import" reasoning db.EventEnvelope already established
// for portal/jobs's own event-listener decoding. Any value outside
// this set is rejected outright, never silently clamped to the
// nearest valid one — same "reject, don't guess" convention this tool
// already uses for other enumerated inputs (e.g. crawl_runs.kind/
// status). See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md and
// plan/ai/tools/career/step-85-auto-discovery-per-link-custom-interval.md.
var AllowedAutoDiscoveryIntervalMinutes = map[int]bool{
	15: true, 30: true, 60: true, 180: true, 360: true, 720: true, 1440: true,
}
