import { Carousel } from '../Carousel/Carousel'
import type { Orientation } from '../Carousel/carouselRules'
import { PAGE_TIPS, type PageTipsId } from './pageTipData'
import { slidesFor } from './pageTipSlides'
import { tipLayoutFor } from './tipLayout'

// The tips of one Career page (steps 105 to 107). Horizontal (the default) is a coloured banner for a wide area, to
// put right under the page's header: <PageTips page="jobs" />. Vertical is for a narrow column of its own, beside
// the page's content: <PageTips page="cvs" orientation="vertical" />, with each tip stacked (badge above the words).
// The text is in pageTipData.ts.
export function PageTips({ page, orientation = 'horizontal' }: { page: PageTipsId; orientation?: Orientation }) {
  return (
    <div className={`career-enter career-delay-1 ${orientation === 'horizontal' ? 'mb-5' : ''}`}>
      <Carousel orientation={orientation} ariaLabel={`Tips: ${PAGE_TIPS[page].name}`} slides={slidesFor(page, tipLayoutFor(orientation))} />
    </div>
  )
}
