import { Carousel } from '../Carousel/Carousel'
import { AUTO_PLAY_MS, type Orientation } from '../Carousel/carouselRules'
import { PAGE_TIPS, type PageTipsId } from './pageTipData'
import { slidesFor } from './pageTipSlides'
import { tipLayoutFor } from './tipLayout'

// The tips of one Career page (steps 105 to 109). Horizontal (the default) is a coloured banner for a wide area, to
// put right under the page's header: <PageTips page="jobs" />. Vertical is for a narrow column of its own, beside
// the page's content: <PageTips page="cvs" orientation="vertical" />, with each tip stacked (badge above the words).
// They move on by themselves every 5000 ms and go round (AUTO_PLAY_MS has 3000 and 8000 as well; pass 0 to switch
// it off for one page). The slider pauses while the pointer or focus is on it, and has a pause button.
// The text is in pageTipData.ts.
export function PageTips({
  page,
  orientation = 'horizontal',
  autoPlayMs = AUTO_PLAY_MS.normal,
}: {
  page: PageTipsId
  orientation?: Orientation
  autoPlayMs?: number
}) {
  return (
    <div className={`career-enter career-delay-1 ${orientation === 'horizontal' ? 'mb-5' : ''}`}>
      <Carousel
        orientation={orientation}
        ariaLabel={`Tips: ${PAGE_TIPS[page].name}`}
        slides={slidesFor(page, tipLayoutFor(orientation))}
        autoPlayMs={autoPlayMs}
        loop={autoPlayMs > 0}
      />
    </div>
  )
}
