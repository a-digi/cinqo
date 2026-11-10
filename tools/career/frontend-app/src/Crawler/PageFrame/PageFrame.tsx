import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'

import { fetchSnapshotHtml } from '../linkCrawl'
import { BLOCK_CLASS, HIDE_FIXED_STYLE, HIDE_IMAGES_STYLE } from '../pageView'

// About the width of the bar, to keep it inside the view.
const BAR_WIDTH = 400
const BLOCKISH = /^(block|flex|grid|list-item|table|flow-root)/

// The bar a click or a selection opens: what it is about (the owner's own ids: a link's address, an element's number), whether it came from a click, and where
// it goes in the frame.
export interface FrameBar {
  targets: string[]
  click: boolean
  left: number
  top: number
}

// The page of a job as the Browser tool read it (plan 247, shared by the Job list and the Job details pickers since plan 249): a copy with no script in it,
// shown in a sandboxed iframe that runs none either. The page scrolls on its own, pop-ups and fixed bars can be hidden, and so can the images. The frame knows
// nothing about links or fields: its owner says which numbers the copy carries (`attribute`), what to draw on them (`marks`), what a click or a selection
// means (`clickTargets`, `onSelection`) and what the bar looks like (`renderBar`); the frame loads, wires, draws, scrolls and places the bar.
export function PageFrame({
  snapshotId,
  large,
  testId,
  hint,
  attribute,
  style,
  marks,
  markClasses,
  reveal,
  onReady,
  clickTargets,
  onSelection,
  renderBar,
}: {
  snapshotId: string
  // The list beside the page is hidden: the page takes the whole width and a taller frame.
  large: boolean
  testId: string
  // The sentence in the header that says what to do.
  hint: string
  // The attribute of the copy that carries the numbers (data-cinqo-a on links, data-cinqo-e on elements).
  attribute: string
  // The css the frame adds to the copy: its own (scrolling, selection) and the owner's marks.
  style: string
  // The class to draw on each number, and every class the marks use (they are cleared before drawing).
  marks: Map<number, string>
  markClasses: string[]
  // Scroll the first of these numbers that is in the picture into view; seq makes asking twice for the same ones work.
  reveal: { numbers: number[]; seq: number } | null
  // The numbers that are in the picture.
  onReady: (present: Set<number>) => void
  // What a click on this element is about ([] = nothing to ask). A click on a link never opens it, whatever this says.
  clickTargets: (target: Element, doc: Document) => string[]
  // What a selection is about ([] = nothing); the owner may act on it here (the Job list ticks the links as jobs).
  onSelection: (range: Range, doc: Document) => string[]
  renderBar: (bar: FrameBar, style: CSSProperties, close: () => void) => ReactNode
}) {
  const [html, setHtml] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [hideImages, setHideImages] = useState(false)
  const [hideFixed, setHideFixed] = useState(true)
  const [fixedCount, setFixedCount] = useState(0)
  const [loaded, setLoaded] = useState(0)
  const [bar, setBar] = useState<FrameBar | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const frame = useRef<HTMLIFrameElement>(null)
  // The handlers added to the copy's document live as long as it does, so they read the latest of the owner's functions through this.
  const latest = useRef({ clickTargets, onSelection, style, attribute })
  latest.current = { clickTargets, onSelection, style, attribute }

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
      ['cinqo-view', style],
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
    onReady(new Set(Array.from(d.querySelectorAll(`[${attribute}]`), (e) => Number(e.getAttribute(attribute)))))
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
      if (typeof target?.closest !== 'function') return
      if (target.closest('a')) e.preventDefault() // a link never opens here
      if (selectionOf()) return // the end of a selection, which has been handled
      const targets = latest.current.clickTargets(target as Element, d)
      if (targets.length === 0) return
      // What it is is the person's to say: the bar asks, where they clicked, and nothing is marked until they answer.
      const f = frame.current?.getBoundingClientRect()
      const b = box.current?.getBoundingClientRect()
      if (!f || !b) return
      setBar({
        targets,
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
      const targets = latest.current.onSelection(range, d)
      if (targets.length === 0) {
        setBar(null)
        return
      }
      const r = range.getBoundingClientRect()
      const f = frame.current?.getBoundingClientRect()
      const b = box.current?.getBoundingClientRect()
      if (!f || !b) return
      setBar({
        targets,
        click: false,
        left: place(f.left - b.left + r.left, b.width),
        top: Math.max(8, Math.min(f.top - b.top + r.bottom + 8, b.height - 52)),
      })
    })
  }

  // The owner is done with the bar: the selection goes and so does the bar.
  function close() {
    doc()?.defaultView?.getSelection()?.removeAllRanges()
    setBar(null)
  }

  // The marks of the owner, drawn on the page.
  useEffect(() => {
    const d = doc()
    if (!d || loaded === 0) return
    const all = [...markClasses, BLOCK_CLASS]
    for (const el of Array.from(d.querySelectorAll(all.map((c) => `.${c}`).join(',')))) el.classList.remove(...all)
    const win = d.defaultView
    for (const [n, cls] of marks) {
      const el = d.querySelector(`[${attribute}="${String(n)}"]`)
      if (!el) continue
      el.classList.add(cls)
      // An element that is inline but wraps blocks shows no outline or background where the blocks are: a marked one is made a block so its mark shows.
      if (
        win?.getComputedStyle(el).display === 'inline' &&
        Array.from(el.children).some((c) => BLOCKISH.test(win.getComputedStyle(c).display))
      )
        el.classList.add(BLOCK_CLASS)
    }
  }, [marks, markClasses, attribute, loaded])

  useEffect(() => {
    const d = doc()
    const images = d?.getElementById('cinqo-view-images')
    if (images) images.textContent = hideImages ? HIDE_IMAGES_STYLE : ''
    const fixed = d?.getElementById('cinqo-view-fixed')
    if (fixed) fixed.textContent = hideFixed ? HIDE_FIXED_STYLE : ''
  }, [hideImages, hideFixed, loaded])

  // Something ticked or chosen elsewhere is brought into view.
  useEffect(() => {
    const d = doc()
    if (!d || !reveal) return
    for (const n of reveal.numbers) {
      const el = d.querySelector(`[${latest.current.attribute}="${String(n)}"]`)
      const win = d.defaultView
      if (el && win) {
        const top = el.getBoundingClientRect().top + win.scrollY - win.innerHeight / 3
        win.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
        return
      }
    }
  }, [reveal])

  return (
    <div
      ref={box}
      className={`relative flex flex-col overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm ${large ? 'h-[max(34rem,calc(100vh-11rem))]' : 'h-[34rem]'}`}
      data-testid={testId}
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs text-gray-600">
        <span>{hint}</span>
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
      {bar && renderBar(bar, { left: bar.left, top: bar.top }, close)}
    </div>
  )
}
