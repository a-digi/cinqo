import {
  BriefcaseIcon,
  BuildingIcon,
  CrawlInstructionsIcon,
  EyeIcon,
  FilterIcon,
  GlobeIcon,
  LinkGlyphIcon,
  MatchIcon,
  PulseIcon,
  RobotIcon,
  TrashIcon,
  UsersIcon,
} from '../Icons/icons'
import type { TimelineIcon } from './timelineRules'

// The picture of a step. The icons come in different sizes, so the svg inside is sized by the wrapper: one and a half times the font size
// of the place it is in (24 px where the text is 16 px). It is white by default (inside a pin); pass className="text-inherit" to take the
// colour of the surrounding text instead.
export function StepIcon({ name, className = 'text-white' }: { name: TimelineIcon; className?: string }) {
  return <span className={`inline-flex ${className} [&>svg]:h-[1.5em] [&>svg]:w-[1.5em]`}>{iconFor(name)}</span>
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
    case 'building':
      return <BuildingIcon />
    case 'users':
      return <UsersIcon />
    case 'link':
      return <LinkGlyphIcon />
    case 'eye':
      return <EyeIcon />
    case 'briefcase':
      return <BriefcaseIcon />
  }
}
