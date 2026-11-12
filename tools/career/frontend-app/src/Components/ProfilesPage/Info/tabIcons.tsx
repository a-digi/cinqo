import {
  BriefcaseIcon,
  CheckCircleIcon,
  HelpIcon,
  LayersIcon,
  PulseIcon,
  TargetIcon,
  TimelineIcon,
  UserIcon,
} from '../../../Shared/Icons/icons'
import type { TabIcon } from './profileLandingData'

// The line icon of a tab of the landing look: each key of the layout data is one of the tool's own icons.
export function tabIconFor(key: TabIcon): React.ReactNode {
  const c = 'h-7 w-7'
  switch (key) {
    case 'flow':
      return <PulseIcon className={c} />
    case 'user':
      return <UserIcon className={c} />
    case 'target':
      return <TargetIcon className={c} />
    case 'layers':
      return <LayersIcon className={c} />
    case 'gauge':
      return <CheckCircleIcon className={c} />
    case 'briefcase':
      return <BriefcaseIcon className={c} />
    case 'steps':
      return <TimelineIcon className={c} />
    case 'help':
      return <HelpIcon className={c} />
  }
}
