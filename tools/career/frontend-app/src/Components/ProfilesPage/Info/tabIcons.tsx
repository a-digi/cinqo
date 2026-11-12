import {
  BoltIcon,
  BriefcaseIcon,
  CameraIcon,
  CheckCircleIcon,
  CoursesIcon,
  EducationIcon,
  EyeIcon,
  HelpIcon,
  HobbiesIcon,
  LanguagesIcon,
  LayersIcon,
  LinkGlyphIcon,
  LockIcon,
  MatchIcon,
  PDFIcon,
  PencilIcon,
  PlusIcon,
  PulseIcon,
  RobotIcon,
  SparkIcon,
  TargetIcon,
  TimelineIcon,
  UploadIcon,
  UserIcon,
} from '../../../Shared/Icons/icons'
import type { TabIcon } from './profileLandingData'

// The line icon of a tab or a tile of the landing look: each key of the layout data is one of the tool's own icons.
export function iconFor(key: TabIcon, c = 'h-7 w-7'): React.ReactNode {
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
    case 'bolt':
      return <BoltIcon className={c} />
    case 'link':
      return <LinkGlyphIcon className={c} />
    case 'pencil':
      return (
        <span className="inline-flex [&>svg]:h-6 [&>svg]:w-6">
          <PencilIcon />
        </span>
      )
    case 'education':
      return <EducationIcon className={c} />
    case 'languages':
      return <LanguagesIcon className={c} />
    case 'courses':
      return <CoursesIcon className={c} />
    case 'hobbies':
      return <HobbiesIcon className={c} />
    case 'camera':
      return <CameraIcon className={c} />
    case 'match':
      return (
        <span className="inline-flex [&>svg]:h-6 [&>svg]:w-6">
          <MatchIcon />
        </span>
      )
    case 'pdf':
      return <PDFIcon className={c} />
    case 'upload':
      return <UploadIcon className={c} />
    case 'robot':
      return <RobotIcon className={c} />
    case 'users':
      return <UserIcon className={c} />
    case 'eye':
      return (
        <span className="inline-flex [&>svg]:h-6 [&>svg]:w-6">
          <EyeIcon />
        </span>
      )
    case 'spark':
      return <SparkIcon className={c} />
    case 'lock':
      return <LockIcon className={c} />
    case 'plus':
      return (
        <span className="inline-flex [&>svg]:h-6 [&>svg]:w-6">
          <PlusIcon />
        </span>
      )
  }
}

export function tabIconFor(key: TabIcon): React.ReactNode {
  return iconFor(key)
}
