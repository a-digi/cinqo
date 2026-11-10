// pageView.ts (plan 247, step 3) — the pure parts of the page view of the Job list picker: which anchor of the page copy gets which mark, and the style
// the view injects into the copy.
import type { Marks } from './linkCrawlRules'
import type { PageLink } from './linkCrawl'

export type AnchorMark = 'job' | 'notJob' | 'next'

// The class an anchor of the copy carries for its mark.
export const MARK_CLASS: Record<AnchorMark, string> = { job: 'cinqo-job', notJob: 'cinqo-notjob', next: 'cinqo-next' }

// An anchor that is inline but wraps blocks (a card, a heading) shows no outline or background where the blocks are; a marked one is made a block so its mark shows.
export const BLOCK_CLASS = 'cinqo-block'

// The anchors of each link of the list, by the link's address (the picker's marks are by address).
export function anchorsByUrl(links: PageLink[]): Map<string, number[]> {
  const out = new Map<string, number[]>()
  for (const l of links) if (l.anchors && l.anchors.length > 0) out.set(l.url, l.anchors)
  return out
}

// The mark of every anchor number that a marked link owns. A link has one mark (Marks keeps it so), and every anchor of the link carries it.
export function anchorMarks(marks: Marks, anchors: Map<string, number[]>): Map<number, AnchorMark> {
  const out = new Map<number, AnchorMark>()
  const put = (urls: string[], mark: AnchorMark) => {
    for (const u of urls) for (const n of anchors.get(u) ?? []) out.set(n, mark)
  }
  put(marks.pages, 'next')
  put(marks.notJob, 'notJob')
  put(marks.picked, 'job')
  return out
}

// The style the view adds to the page copy: the marks, and a pointer on the links (a click never leaves the view). The images rule is a second element so it can
// be switched.
export const VIEW_STYLE = `
a[data-cinqo-a] { cursor: pointer; }
a[data-cinqo-a].cinqo-block { display: block !important; }
a[data-cinqo-a].cinqo-job { outline: 3px solid #16a34a; outline-offset: 2px; background-color: rgba(22, 163, 74, 0.16) !important; border-radius: 2px; }
a[data-cinqo-a].cinqo-notjob { outline: 3px solid #dc2626; outline-offset: 2px; background-color: rgba(220, 38, 38, 0.12) !important; text-decoration: line-through !important; border-radius: 2px; }
a[data-cinqo-a].cinqo-next { outline: 3px dashed #111827; outline-offset: 2px; background-color: rgba(17, 24, 39, 0.1) !important; border-radius: 2px; }
`
export const HIDE_IMAGES_STYLE = 'img, picture, video, svg image { visibility: hidden !important; }'
