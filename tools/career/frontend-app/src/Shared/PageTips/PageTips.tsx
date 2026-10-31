import { Carousel } from '../Carousel/Carousel'
import { PAGE_TIPS, type PageTipsId } from './pageTipData'
import { slidesFor } from './pageTipSlides'

// The tips of one Career page, as a coloured horizontal banner (steps 105 and 106). Put it right under the page's
// header: <PageTips page="jobs" />. The text is in pageTipData.ts.
export function PageTips({ page }: { page: PageTipsId }) {
  return (
    <div className="career-enter career-delay-1 mb-5">
      <Carousel orientation="horizontal" ariaLabel={`Tips: ${PAGE_TIPS[page].name}`} slides={slidesFor(page)} />
    </div>
  )
}
