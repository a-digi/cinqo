import { useMemo } from 'react'

import { PageFrame } from '../PageFrame/PageFrame'
import type { PageLink } from '../linkCrawl'
import type { MarkKind, Marks } from '../linkCrawlRules'
import { anchorMarks, anchorsByUrl, linksInRange, MARK_CLASS, urlByAnchor, VIEW_STYLE } from '../pageView'

const MARK_CLASSES = Object.values(MARK_CLASS)
const CHOICES: { kind: MarkKind | null; label: string; tone: string }[] = [
  { kind: 'picked', label: 'Job', tone: 'text-green-700 hover:bg-green-50' },
  { kind: 'notJob', label: 'Not a job', tone: 'text-red-700 hover:bg-red-50' },
  { kind: 'pages', label: 'Next page', tone: 'text-gray-700 hover:bg-gray-100' },
  { kind: null, label: 'Clear', tone: 'text-gray-700 hover:bg-gray-100' },
]

// The page of the link as the Browser tool read it (plan 247): a copy with no script in it, shown in a sandboxed iframe that runs none either. The page
// scrolls on its own. The panel's marks (jobs, not a job, next page) are drawn on the links of the page, and a link ticked in the list is scrolled into
// view. The other way round: a click on a link asks what it is (job, not a job, next page, or clear) in a small bar, and selecting text ticks every link the
// selection touches as a job, with the same bar to change them. The panel's list stays the one place the marks live; this only shows them and reports what the person did.
// The frame itself (loading, scrolling, the bar's place, hiding pop-ups) is PageFrame, shared with the Job details picker since plan 249.
export function PageView({
  snapshotId,
  links,
  marks,
  reveal,
  onReady,
  large,
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
  // The list beside the page is hidden: the page takes the whole width and a taller frame.
  large: boolean
  // Whether a link of the page may be marked (a link of another site cannot be: a rule is for the link's own site).
  selectable: (url: string) => boolean
  // A selection ticked these links (kind 'picked'), or the bar set what they are (any kind, or null = clear).
  onMark: (urls: string[], kind: MarkKind | null) => void
}) {
  const byUrl = useMemo(() => anchorsByUrl(links), [links])
  const byAnchor = useMemo(() => urlByAnchor(links), [links])
  const drawn = useMemo(() => {
    const out = new Map<number, string>()
    for (const [n, mark] of anchorMarks(marks, byUrl)) out.set(n, MARK_CLASS[mark])
    return out
  }, [marks, byUrl])
  const revealed = useMemo(() => (reveal ? { numbers: byUrl.get(reveal.url) ?? [], seq: reveal.seq } : null), [reveal, byUrl])

  // What these links are now, when they are all the same (null = none, undefined = mixed).
  function current(urls: string[]): MarkKind | null | undefined {
    const of = (u: string): MarkKind | null =>
      marks.picked.includes(u) ? 'picked' : marks.notJob.includes(u) ? 'notJob' : marks.pages.includes(u) ? 'pages' : null
    const first = of(urls[0] ?? '')
    return urls.every((u) => of(u) === first) ? first : undefined
  }

  return (
    <PageFrame
      snapshotId={snapshotId}
      large={large}
      testId="link-crawl-page-view"
      hint="Scroll the page, then click a link to say what it is, or select text to tick its links."
      attribute="data-cinqo-a"
      style={VIEW_STYLE}
      marks={drawn}
      markClasses={MARK_CLASSES}
      reveal={revealed}
      onReady={onReady}
      clickTargets={(target) => {
        const a = target.closest('a')
        const url = a ? byAnchor.get(Number(a.getAttribute('data-cinqo-a'))) : undefined
        return url !== undefined && selectable(url) ? [url] : []
      }}
      onSelection={(range, d) => {
        const urls = linksInRange(range, d, byAnchor).filter((u) => selectable(u))
        if (urls.length > 0) onMark(urls, 'picked') // a selection ticks the links it touches as jobs at once
        return urls
      }}
      renderBar={(bar, style, close) => (
        <div
          role="toolbar"
          aria-label={bar.click ? 'What this link is' : 'What the selected links are'}
          data-testid="page-view-bar"
          style={style}
          className="absolute z-10 flex items-center gap-1 rounded-xl border border-gray-200 bg-white px-2 py-1.5 text-xs shadow-lg"
        >
          <span className="px-1 font-medium text-gray-900">
            {bar.click ? 'This link is' : `${String(bar.targets.length)} link${bar.targets.length === 1 ? '' : 's'} ticked`}
          </span>
          {CHOICES.map((c) => (
            <button
              key={c.label}
              type="button"
              aria-pressed={c.kind !== null && current(bar.targets) === c.kind}
              className={`rounded-md border px-2 py-1 aria-pressed:border-gray-900 aria-pressed:bg-gray-50 aria-pressed:font-semibold ${c.tone} border-gray-200`}
              onClick={() => {
                onMark(bar.targets, c.kind)
                close()
              }}
            >
              {c.label}
            </button>
          ))}
        </div>
      )}
    />
  )
}
