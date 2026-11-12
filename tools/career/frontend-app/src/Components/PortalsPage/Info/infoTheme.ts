// The colour of each chapter of the infographic (plan 262), from the tool's own palette (jobsTheme), so the poster looks like the pages around it.
import { ACCENTS, type Accent } from '../../JobsPage/jobsTheme'
import type { ChapterId } from './platformInfoData'

const BY_CHAPTER: Record<ChapterId, Accent> = {
  overview: ACCENTS[0], // blue
  platform: ACCENTS[0], // blue
  links: ACCENTS[6], // sky
  crawler: ACCENTS[5], // teal
  instructions: ACCENTS[4], // violet
  'first-steps': ACCENTS[1], // green
  trouble: ACCENTS[2], // amber
}

export function accentOfChapter(id: ChapterId): Accent {
  return BY_CHAPTER[id]
}
