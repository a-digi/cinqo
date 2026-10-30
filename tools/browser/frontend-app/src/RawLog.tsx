import { useEffect, useRef, useState } from 'react'

// "Raw log" under an expanded crawl: the crawl's own file, shown line by line in Cinqo's terminal-style file
// reader (bridge.mountFileReader, the "crawl-logs" source in manifest.json). Left out entirely on a Cinqo that
// predates the file reader. The reader draws itself into the div below and is removed when this closes.
export function RawLog({ id }: { id: string }) {
  const [open, setOpen] = useState(false)
  const host = useRef<HTMLDivElement>(null)
  const bridge = window.__cinqoToolBridge
  const supported = typeof bridge.mountFileReader === 'function'

  useEffect(() => {
    const container = host.current
    if (!open || !container || typeof bridge.mountFileReader !== 'function') return
    return bridge.mountFileReader(container, {
      tool: 'browser',
      source: 'crawl-logs',
      ref: `crawl_logs/${id}.json`,
      title: `crawl-${id.slice(0, 8)}.json`,
    })
  }, [open, id, bridge])

  if (!supported) return null
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          setOpen((v) => !v)
        }}
        aria-expanded={open}
        className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
      >
        {open ? 'hide raw log' : 'raw log'}
      </button>
      {open && <div ref={host} className="mt-2" />}
    </div>
  )
}
