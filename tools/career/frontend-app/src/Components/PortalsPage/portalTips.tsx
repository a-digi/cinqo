import type { CarouselSlide } from '../../Shared/Carousel/Carousel'
import { PORTAL_TIP_DATA, toneAccent } from './portalTipData'
import { PortalTip } from './PortalTip'

// The tips as carousel slides (step 105, coloured in step 106): the text is in portalTipData.ts, one tip's look
// in PortalTip.tsx; each slide carries its tone's accent, so its card, shapes, arrows and dots share the colour.
export const PORTAL_TIPS: CarouselSlide[] = PORTAL_TIP_DATA.map((tip) => ({
  id: tip.id,
  label: tip.label,
  accent: toneAccent(tip.tone),
  content: <PortalTip tip={tip} />,
}))
