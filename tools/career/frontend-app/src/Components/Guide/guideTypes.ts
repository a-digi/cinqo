// The in-code model of a guide page (plan/ai/tools/career/step-170-onboarding-guide.md): plain text and typed data, no markup.
import type { TimelineDesign, TimelineStep } from '../../Shared/Timeline/timelineRules'

export interface Guide {
  // Unique, kept stable (it is the key of the guide in the code).
  id: string
  title: string
  description: string
  // How the steps are drawn; each guide has its own.
  design: TimelineDesign
  steps: TimelineStep[]
}

// One question of the Questions page, with an optional link to the page the answer is about.
export interface Question {
  id: string
  question: string
  answer: string
  link?: { label: string; path: string }
}
