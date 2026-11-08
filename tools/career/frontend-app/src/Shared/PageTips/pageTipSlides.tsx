import type { CarouselSlide } from '../Carousel/Carousel'
import { PAGE_TIPS, toneAccent, type PageTipsId } from './pageTipData'
import { PageTipBody } from './PageTipBody'
import type { TipLayout } from './tipLayout'

// A page's tips as carousel slides, built once per page and layout: the carousel keeps the same array between renders.
const cache = new Map<string, CarouselSlide[]>()

export function slidesFor(page: PageTipsId, layout: TipLayout = 'row'): CarouselSlide[] {
  const key = `${page}:${layout}`
  const cached = cache.get(key)
  if (cached) return cached
  const slides = PAGE_TIPS[page].tips.map((tip) => ({
    id: tip.id,
    label: tip.label,
    accent: toneAccent(tip.tone),
    content: <PageTipBody tip={tip} layout={layout} />,
  }))
  cache.set(key, slides)
  return slides
}
