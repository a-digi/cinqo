// The look of each card on a job's page (step 146): which accent colour and which decorative shape it has. Fixed per card,
// so a card keeps its colour whatever else is on the page. Pure (no React), so it can be script-checked.
import { ACCENTS, PAGE_ACCENT, type Accent } from '../JobsPage/jobsTheme'
import { shapeAt, type DecorShape } from '../../Shared/Decor/decorShapes'

export type CardKey = 'match' | 'persona' | 'board' | 'cv' | 'requirements' | 'recruiters' | 'protocol'

export interface CardLook {
  accent: Accent
  shape: DecorShape
}

// Blue (the page's own colour), violet, teal, amber, and orange from the shared palette; the shapes take turns in the cards' order
// (the right column first, then Requirements on the left; the host draws the comments panel in its own colours).
export const CARD_LOOKS: Record<CardKey, CardLook> = {
  match: { accent: PAGE_ACCENT, shape: shapeAt(0) },
  persona: { accent: ACCENTS[4], shape: shapeAt(1) },
  board: { accent: ACCENTS[5], shape: shapeAt(2) },
  cv: { accent: ACCENTS[2], shape: shapeAt(3) },
  requirements: { accent: ACCENTS[7], shape: shapeAt(4) },
  // The Recruiters card (plan/ai/tools/career/step-168): sky blue, the next shape in turn.
  recruiters: { accent: ACCENTS[6], shape: shapeAt(5) },
  // The Protocol card (plan/ai/tools/career/step-176): rose, the next shape in turn.
  protocol: { accent: ACCENTS[3], shape: shapeAt(6) },
}
