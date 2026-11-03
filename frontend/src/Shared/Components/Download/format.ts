// How sizes, speeds and times are put into words, shared by everything that shows a download. Pure (no React), so the rules
// can be script-checked.

const KIB = 1024
const MIB = KIB ** 2
const GIB = KIB ** 3

// A size the way an operating system shows memory: binary units, labelled so nobody has to guess. A size we do not know
// (zero, negative or not a number) is said to be unknown.
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return 'unknown'
  if (bytes >= GIB) {
    const gib = bytes / GIB
    return `${gib >= 10 ? String(Math.round(gib)) : gib.toFixed(1)} GiB`
  }
  if (bytes >= MIB) {
    const mib = bytes / MIB
    return `${mib >= 10 ? String(Math.round(mib)) : mib.toFixed(1)} MiB`
  }
  if (bytes >= KIB) return `${String(Math.round(bytes / KIB))} KiB`
  return `${String(Math.round(bytes))} B`
}

// A speed such as "12.4 MiB/s"; nothing is shown while it is not known.
export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return ''
  return `${formatBytes(bytesPerSecond)}/s`
}

// "about 20 seconds", "about 3 minutes", "about 1 h 5 min". Always rounded up, so the promise is never too short; null means
// it cannot be told yet.
export function formatTimeLeft(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds) || seconds < 0) return ''
  const s = Math.ceil(seconds)
  if (s < 60) return `about ${String(Math.max(s, 1))} ${s <= 1 ? 'second' : 'seconds'}`
  if (s < 3600) {
    const m = Math.ceil(s / 60)
    return `about ${String(m)} ${m === 1 ? 'minute' : 'minutes'}`
  }
  const h = Math.floor(s / 3600)
  const m = Math.ceil((s - h * 3600) / 60)
  if (m === 60) return `about ${String(h + 1)} h`
  return m === 0 ? `about ${String(h)} h` : `about ${String(h)} h ${String(m)} min`
}

// The share done, 0 to 100, rounded down so "100%" only ever means finished; 0 when the total is unknown.
export function percent(done: number, total: number): number {
  if (!Number.isFinite(done) || !Number.isFinite(total) || total <= 0 || done <= 0) return 0
  return Math.min(100, Math.floor((done / total) * 100))
}
