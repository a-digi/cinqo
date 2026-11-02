// The in-code model of a guide page (plan/ai/tools/career/step-170-onboarding-guide.md): plain text and typed data, no markup.
import type { TimelineStep } from '../../Shared/Timeline/timelineRules'

export interface Guide {
  // Unique, kept stable (it is the key of the guide in the code).
  id: string
  title: string
  description: string
  steps: TimelineStep[]
}
