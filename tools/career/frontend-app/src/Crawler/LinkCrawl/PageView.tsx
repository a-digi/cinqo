import { useEffect, useRef, useState } from 'react'

import { fetchSnapshotHtml, type PageLink } from '../linkCrawl'
import type { MarkKind, Marks } from '../linkCrawlRules'
import {
  anchorMarks,
  anchorsByUrl,
  BLOCK_CLASS,
  HIDE_FIXED_STYLE,
  HIDE_IMAGES_STYLE,
  linksInRange,
  MARK_CLASS,
  urlByAnchor,
  VIEW_STYLE,
} from '../pageView'

const ALL_MARK_CLASSES = [...Object.values(MARK_CLASS), BLOCK_CLASS]
// about the width of the bar, to keep it inside the view
const BAR_WIDTH = 400
const CHOICES: { kind: MarkKind | null; label: string; tone: string }[] = [
  { kind: 'picked', label: 'Job', tone: 'text-green-700 hover:bg-green-50' },
  { kind: 'notJob', label: 'Not a job', tone: 'text-red-700 hover:bg-red-50' },
  { kind: 'pages', label: 'Next page', tone: 'text-gray-700 hover:bg-gray-100' },
  { kind: null, label: 'Clear', tone: 'text-gray-700 hover:bg-gray-100' },
]
const BLOCKISH = /^(block|flex|grid|list-item|table|flow-root)/

// The page of the link as the Browser tool read it (plan 247): a copy with no script in it, shown in a sandboxed iframe that runs none either. The page
// scrolls on its own. The panel's marks (jobs, not a job, next page) are drawn on the links of the page, and a link ticked in the list is scrolled into
// view. The other way round: a click on a link asks what it is (job, not a job, next page, or clear) in a small bar, and selecting text ticks every link the
// selection touches as a job, with the same bar to change them. The panel's list stays the one place the marks live; this only shows them and reports what the person did.
export function PageView({
  snapshotId,
  links,
  marks,
  reveal,
  onReady,
  selectable,
  onMark,
}: {
  snapshotId: string
  links: PageLink[]
  marks: Marks
  // Scroll the first anchor of this link into view; seq makes asking twice for the same link work.
  reveal: { url: string; seq: number } | null
  // The anchor numbers that are in the picture (a link inside a shadow root is listed but not drawn).
  onReady: (present: Set<number>) => void
  // Whether a link of the page may be marked (a link of another site cannot be: a rule is for the link's own site).
  selectable: (url: string) => boolean
  // A selection ticked these links (kind 'picked'), or the bar set what they are (any kind, or null = clear).
  onMark: (urls: string[], kind: MarkKind | null) => void
}) {
  const [html, setHtml] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [hideImages, setHideImages] = useState(false)
  const [hideFixed, setHideFixed] = useState(true)
  const [fixedCount, setFixedCount] = useState(0)
  const [loaded, setLoaded] = useState(0)
  const [bar, setBar] = useState<{ urls: string[]; left: number; top: number; click: boolean } | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const frame = useRef<HTMLIFrameElement>(null)
  const byUrl = useRef(new Map<string, number[]>())
  const byAnchor = useRef(new Map<number, string>())
  byUrl.current = anchorsByUrl(links)
  byAnchor.current = urlByAnchor(links)
  // The handlers added to the copy's document live as long as it does, so they read the latest props through this.
  const latest = useRef({ selectable, onMark })
  latest.current = { selectable, onMark }

  useEffect(() => {
    const ctl = new AbortController()
    setHtml(null)
    setError('')
    setBar(null)
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
      ['cinqo-view-fixed', ''],
    ] as const) {
      let el = d.getElementById(id)
      if (!el) {
        el = d.createElement('style')
        el.id = id
        d.head.appendChild(el)
      }
      el.textContent = css
    }
    // Pop-ups, cookie bars and fixed headers cover the page and follow the scroll; they are found once so they can be hidden.
    const win = d.defaultView
    let fixed = 0
    if (win) {
      for (const el of Array.from(d.body.querySelectorAll('*'))) {
        if (win.getComputedStyle(el).position === 'fixed') {
          el.setAttribute('data-cinqo-fixed', '1')
          fixed += 1
        }
      }
    }
    setFixedCount(fixed)
    wire(d)
    onReady(new Set(Array.from(d.querySelectorAll('a[data-cinqo-a]'), (a) => Number(a.getAttribute('data-cinqo-a')))))
    setLoaded((n) => n + 1)
  }

  // The bar stays inside the view.
  const place = (left: number, width: number) => Math.max(8, Math.min(left, width - BAR_WIDTH))

  // Clicks and selections in the page.
  function wire(d: Document) {
    // A copy is wired once: the marker sits on the document itself, so a new document (a new copy) is wired again and the same one is not.
    if (d.documentElement.hasAttribute('data-cinqo-wired')) return
    d.documentElement.setAttribute('data-cinqo-wired', '1')
    const selectionOf = () => {
      const s = d.defaultView?.getSelection()
      return s && !s.isCollapsed && s.rangeCount > 0 ? s : null
    }
    d.addEventListener('click', (e) => {
      // not `instanceof Element`: the page's elements belong to the iframe's window, whose Element is not ours
      const target = e.target as Partial<Element> | null
      const a = typeof target?.closest === 'function' ? target.closest('a') : null
      if (!a) return
      e.preventDefault() // a link never opens here
      if (selectionOf()) return // the end of a selection, which has been handled
      const url = byAnchor.current.get(Number(a.getAttribute('data-cinqo-a')))
      if (url === undefined || !latest.current.selectable(url)) return
      // What the link is is the person's to say: the bar asks, where they clicked, and nothing is marked until they answer.
      const f = frame.current?.getBoundingClientRect()
      const b = box.current?.getBoundingClientRect()
      if (!f || !b) return
      setBar({
        urls: [url],
        click: true,
        left: place(f.left - b.left + e.clientX - 20, b.width),
        top: Math.max(8, Math.min(f.top - b.top + e.clientY + 14, b.height - 52)),
      })
    })
    // A drag that starts on a link or an image would be a browser drag-and-drop of it; in this view a drag is a text selection.
    d.addEventListener('dragstart', (e) => {
      e.preventDefault()
    })
    d.addEventListener('mousedown', () => {
      setBar(null)
    })
    d.addEventListener(
      'scroll',
      () => {
        setBar(null)
      },
      true,
    )
    d.addEventListener('mouseup', () => {
      const s = selectionOf()
      if (!s) return
      const range = s.getRangeAt(0)
      const urls = linksInRange(range, d, byAnchor.current).filter((u) => latest.current.selectable(u))
      if (urls.length === 0) {
        setBar(null)
        return
      }
      latest.current.onMark(urls, 'picked')
      const r = range.getBoundingClientRect()
      const f = frame.current?.getBoundingClientRect()
      const b = box.current?.getBoundingClientRect()
      if (!f || !b) return
      setBar({
        urls,
        click: false,
        left: place(f.left - b.left + r.left, b.width),
        top: Math.max(8, Math.min(f.top - b.top + r.bottom + 8, b.height - 52)),
      })
    })
  }

  function markFromBar(kind: MarkKind | null) {
    if (!bar) return
    onMark(bar.urls, kind)
    doc()?.defaultView?.getSelection()?.removeAllRanges()
    setBar(null)
  }

  // What these links are now, when they are all the same (null = none, undefined = mixed).
  function current(urls: string[]): MarkKind | null | undefined {
    const of = (u: string): MarkKind | null =>
      marks.picked.includes(u) ? 'picked' : marks.notJob.includes(u) ? 'notJob' : marks.pages.includes(u) ? 'pages' : null
    const first = of(urls[0] ?? '')
    return urls.every((u) => of(u) === first) ? first : undefined
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
    const d = doc()
    const images = d?.getElementById('cinqo-view-images')
    if (images) images.textContent = hideImages ? HIDE_IMAGES_STYLE : ''
    const fixed = d?.getElementById('cinqo-view-fixed')
    if (fixed) fixed.textContent = hideFixed ? HIDE_FIXED_STYLE : ''
  }, [hideImages, hideFixed, loaded])

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
      ref={box}
      className="relative flex h-[34rem] flex-col overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm"
      data-testid="link-crawl-page-view"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs text-gray-600">
        <span>Scroll the page, then click a link to say what it is, or select text to tick its links.</span>
        <span className="flex items-center gap-3">
          {fixedCount > 0 && (
            <label className="flex items-center gap-1">
              <input
                type="checkbox"
                checked={hideFixed}
                onChange={(e) => {
                  setHideFixed(e.target.checked)
                }}
                className="accent-gray-900"
              />
              Hide pop-ups and bars ({fixedCount})
            </label>
          )}
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
        </span>
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
      {bar && (
        <div
          role="toolbar"
          aria-label={bar.click ? 'What this link is' : 'What the selected links are'}
          data-testid="page-view-bar"
          style={{ left: bar.left, top: bar.top }}
          className="absolute z-10 flex items-center gap-1 rounded-xl border border-gray-200 bg-white px-2 py-1.5 text-xs shadow-lg"
        >
          <span className="px-1 font-medium text-gray-900">
            {bar.click ? 'This link is' : `${String(bar.urls.length)} link${bar.urls.length === 1 ? '' : 's'} ticked`}
          </span>
          {CHOICES.map((c) => (
            <button
              key={c.label}
              type="button"
              aria-pressed={c.kind !== null && current(bar.urls) === c.kind}
              className={`rounded-md border px-2 py-1 aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold ${c.tone} border-gray-200`}
              onClick={() => {
                markFromBar(c.kind)
              }}
            >
              {c.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
