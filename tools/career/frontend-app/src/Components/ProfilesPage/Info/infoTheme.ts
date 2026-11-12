// The colour of each chapter of the profiles infographic (plan 266), from the tool's own palette (jobsTheme), so the poster looks like the pages around it.
import { ACCENTS, type Accent } from '../../JobsPage/jobsTheme'
import type { ChapterId } from './profileInfoData'

const BY_CHAPTER: Record<ChapterId, Accent> = {
  overview: ACCENTS[0], // blue
  profile: ACCENTS[6], // sky
  roles: ACCENTS[4], // violet
  data: ACCENTS[2], // amber
  completeness: ACCENTS[1], // green
  jobs: ACCENTS[5], // teal
  'first-steps': ACCENTS[7], // orange
  careful: ACCENTS[3], // rose
}

export function accentOfChapter(id: ChapterId): Accent {
  return BY_CHAPTER[id]
}
