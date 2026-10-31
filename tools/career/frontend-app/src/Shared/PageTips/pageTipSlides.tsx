import type { CarouselSlide } from '../Carousel/Carousel'
import { PAGE_TIPS, toneAccent, type PageTipsId } from './pageTipData'
import { PageTipBody } from './PageTipBody'

// A page's tips as carousel slides, built once per page: the carousel keeps the same array between renders.
const cache = new Map<PageTipsId, CarouselSlide[]>()

export function slidesFor(page: PageTipsId): CarouselSlide[] {
  const cached = cache.get(page)
  if (cached) return cached
  const slides = PAGE_TIPS[page].tips.map((tip) => ({
    id: tip.id,
    label: tip.label,
    accent: toneAccent(tip.tone),
    content: <PageTipBody tip={tip} />,
  }))
  cache.set(page, slides)
  return slides
}
