import {
  AlertCircleIcon,
  BriefcaseIcon,
  BuildingIcon,
  CheckCircleIcon,
  ClockIcon,
  EyeIcon,
  LayersIcon,
  LinkGlyphIcon,
  LockIcon,
  LogIcon,
  MatchIcon,
  PDFIcon,
  TrashIcon,
  UsersIcon,
} from '../../../Shared/Icons/icons'
import type { JobIcon } from './jobInfoData'

// The classes that scale the svg inside the wrapper, one literal string for each size in use (Tailwind only makes a class it can read whole in the source: a string put together at run
// time would never get its style, and the icons of a fixed size of their own would stay tiny).
const SCALED: Record<string, string> = {
  'h-6 w-6': '[&>svg]:h-6 [&>svg]:w-6',
  'h-8 w-8': '[&>svg]:h-8 [&>svg]:w-8',
  'h-10 w-10': '[&>svg]:h-10 [&>svg]:w-10',
}

// The tool's icon for a key, at one size. Some of the tool's icons have a fixed size of their own: those are scaled by a wrapper.
export function jobIconFor(key: JobIcon, size = 'h-8 w-8'): React.ReactNode {
  const scaled = (icon: React.ReactNode): React.ReactNode => <span className={`inline-flex ${SCALED[size] ?? ''}`}>{icon}</span>
  switch (key) {
    case 'title':
      return <BuildingIcon className={size} />
    case 'details':
      return scaled(<LogIcon />)
    case 'match':
      return scaled(<MatchIcon />)
    case 'cv':
      return <PDFIcon className={size} />
    case 'search':
      return scaled(<EyeIcon />)
    case 'board':
      return <BriefcaseIcon className={size} />
    case 'recruiter':
      return <UsersIcon className={size} />
    case 'applied':
      return <CheckCircleIcon className={size} />
    case 'link':
      return <LinkGlyphIcon className={size} />
    case 'clock':
      return <ClockIcon className={size} />
    case 'lock':
      return <LockIcon className={size} />
    case 'limit':
      return scaled(<AlertCircleIcon />)
    case 'trash':
      return scaled(<TrashIcon />)
    case 'copy':
      return <LayersIcon className={size} />
  }
}
