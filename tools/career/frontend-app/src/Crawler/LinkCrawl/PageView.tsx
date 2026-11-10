import { useEffect, useRef, useState } from 'react'

import { fetchSnapshotHtml, type PageLink } from '../linkCrawl'
import type { Marks } from '../linkCrawlRules'
import { anchorMarks, anchorsByUrl, BLOCK_CLASS, HIDE_IMAGES_STYLE, MARK_CLASS, VIEW_STYLE } from '../pageView'

const ALL_MARK_CLASSES = [...Object.values(MARK_CLASS), BLOCK_CLASS]
const BLOCKISH = /^(block|flex|grid|list-item|table|flow-root)/

// The page of the link as the Browser tool read it (plan 247): a copy with no script in it, shown in a sandboxed iframe that runs none either. The
// panel's marks (jobs, not a job, next page) are drawn on the links of the page, and a link ticked in the list is scrolled into view. The panel's list stays
// the one place the marks live; this only shows them.
export function PageView({
  snapshotId,
  links,
  marks,
  reveal,
  onReady,
}: {
  snapshotId: string
  links: PageLink[]
  marks: Marks
  // Scroll the first anchor of this link into view; seq makes asking twice for the same link work.
  reveal: { url: string; seq: number } | null
  // The anchor numbers that are in the picture (a link inside a shadow root is listed but not drawn).
  onReady: (present: Set<number>) => void
}) {
  const [html, setHtml] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [hideImages, setHideImages] = useState(false)
  const [loaded, setLoaded] = useState(0)
  const frame = useRef<HTMLIFrameElement>(null)
  const byUrl = useRef(new Map<string, number[]>())
  byUrl.current = anchorsByUrl(links)

  useEffect(() => {
    const ctl = new AbortController()
    setHtml(null)
    setError('')
    fetchSnapshotHtml(snapshotId, ctl.signal)
      .then(setHtml)
      .catch((err: unknown) => {
        if (ctl.signal.aborted) return
        setError(err instanceof Error ? err.message : 'The page could not be shown.')
      })
    return () => {
      ctl.abort()
    }
  }, [snapshotId])

  function doc(): Document | null {
    return frame.current?.contentDocument ?? null
  }

  function onLoad() {
    const d = doc()
    if (!d?.head) return
    for (const [id, css] of [
      ['cinqo-view', VIEW_STYLE],
      ['cinqo-view-images', ''],
    ] as const) {
      let el = d.getElementById(id)
      if (!el) {
        el = d.createElement('style')
        el.id = id
        d.head.appendChild(el)
      }
      el.textContent = css
    }
    onReady(new Set(Array.from(d.querySelectorAll('a[data-cinqo-a]'), (a) => Number(a.getAttribute('data-cinqo-a')))))
    setLoaded((n) => n + 1)
  }

  // The marks of the panel, drawn on the page.
  useEffect(() => {
    const d = doc()
    if (!d || loaded === 0) return
    for (const el of Array.from(d.querySelectorAll(ALL_MARK_CLASSES.map((c) => `a.${c}`).join(','))))
      el.classList.remove(...ALL_MARK_CLASSES)
    const win = d.defaultView
    for (const [n, mark] of anchorMarks(marks, byUrl.current)) {
      const a = d.querySelector(`a[data-cinqo-a="${String(n)}"]`)
      if (!a) continue
      a.classList.add(MARK_CLASS[mark])
      if (
        win?.getComputedStyle(a).display === 'inline' &&
        Array.from(a.children).some((c) => BLOCKISH.test(win.getComputedStyle(c).display))
      )
        a.classList.add(BLOCK_CLASS)
    }
  }, [marks, links, loaded])

  useEffect(() => {
    const el = doc()?.getElementById('cinqo-view-images')
    if (el) el.textContent = hideImages ? HIDE_IMAGES_STYLE : ''
  }, [hideImages, loaded])

  // A link ticked in the list is brought into view.
  useEffect(() => {
    const d = doc()
    if (!d || !reveal) return
    for (const n of byUrl.current.get(reveal.url) ?? []) {
      const a = d.querySelector(`a[data-cinqo-a="${String(n)}"]`)
      const win = d.defaultView
      if (a && win) {
        const top = a.getBoundingClientRect().top + win.scrollY - win.innerHeight / 3
        win.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
        return
      }
    }
  }, [reveal])

  return (
    <div
      className="flex h-[34rem] flex-col overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm"
      data-testid="link-crawl-page-view"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs text-gray-600">
        <span>The page as it was read. Links do not open here.</span>
        <label className="flex items-center gap-1">
          <input
            type="checkbox"
            checked={hideImages}
            onChange={(e) => {
              setHideImages(e.target.checked)
            }}
            className="accent-gray-900"
          />
          Hide images
        </label>
      </div>
      {error && (
        <p role="alert" className="p-3 text-sm text-red-700">
          {error}
        </p>
      )}
      {!error && html === null && <p className="p-3 text-sm text-gray-500">Loading the page…</p>}
      {html !== null && (
        <iframe
          ref={frame}
          title="The page"
          sandbox="allow-same-origin"
          srcDoc={html}
          onLoad={onLoad}
          className="min-h-0 w-full flex-1 bg-white"
        />
      )}
    </div>
  )
}
