import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import type { NodeLook } from './timelineRules'

// The look of a step's number badge for each state. A step with no state is a filled badge in the step's own colour.
export function badgeStyle(look: NodeLook, accent: Accent): React.CSSProperties {
  switch (look) {
    case 'done':
      return { background: '#16a34a', color: '#ffffff' }
    case 'upcoming':
      return { background: '#ffffff', color: accent.text, border: `2px solid ${accent.from}` }
    case 'current':
      return { background: gradient(accent), color: '#ffffff', boxShadow: `0 0 0 4px ${accent.soft}` }
    case 'neutral':
      return { background: gradient(accent), color: '#ffffff' }
  }
}
