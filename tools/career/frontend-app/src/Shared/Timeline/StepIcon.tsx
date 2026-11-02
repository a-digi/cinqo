import { CrawlInstructionsIcon, FilterIcon, GlobeIcon, MatchIcon, PulseIcon, RobotIcon, TrashIcon } from '../Icons/icons'
import type { TimelineIcon } from './timelineRules'

// The picture of a step (the pins design draws it in white inside the pin). The icons come in different sizes, so the svg inside is sized
// by the wrapper.
export function StepIcon({ name }: { name: TimelineIcon }) {
  return <span className="inline-flex text-white [&>svg]:h-6 [&>svg]:w-6">{iconFor(name)}</span>
}

function iconFor(name: TimelineIcon): React.ReactNode {
  switch (name) {
    case 'robot':
      return <RobotIcon />
    case 'globe':
      return <GlobeIcon />
    case 'instructions':
      return <CrawlInstructionsIcon />
    case 'pulse':
      return <PulseIcon />
    case 'filter':
      return <FilterIcon />
    case 'trash':
      return <TrashIcon />
    case 'match':
      return <MatchIcon />
  }
}
