// How a tip's body is laid out (step 107): in a row for the wide horizontal banner (a badge, then the title and the
// text beside it), stacked for the narrow vertical slider (the badge above, the title and the text under it), because
// a row would leave a narrow column too little room for the text. Pure, so it can be script-checked.
import type { Orientation } from '../Carousel/carouselRules'

export type TipLayout = 'row' | 'stack'

export function tipLayoutFor(orientation: Orientation): TipLayout {
  return orientation === 'vertical' ? 'stack' : 'row'
}
