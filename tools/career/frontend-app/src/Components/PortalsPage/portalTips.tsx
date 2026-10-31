import type { CarouselSlide } from '../../Shared/Carousel/Carousel'
import { PORTAL_TIP_DATA } from './portalTipData'
import { PortalTip } from './PortalTip'

// The tips as carousel slides (step 105): the text is in portalTipData.ts, one tip's look in PortalTip.tsx.
export const PORTAL_TIPS: CarouselSlide[] = PORTAL_TIP_DATA.map((tip) => ({
  id: tip.id,
  label: tip.label,
  content: <PortalTip tip={tip} />,
}))
