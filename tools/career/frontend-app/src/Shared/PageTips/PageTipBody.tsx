import type { ComponentType } from 'react'
import {
  AlertIcon,
  BoltIcon,
  BriefcaseIcon,
  BuildingIcon,
  CameraIcon,
  CheckCircleIcon,
  ClockIcon,
  EyeIcon,
  FilterIcon,
  GlobeIcon,
  LayersIcon,
  LinkGlyphIcon,
  LockIcon,
  LogIcon,
  MatchIcon,
  PDFIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  PulseIcon,
  RobotIcon,
  SparkIcon,
  StopIcon,
  TargetIcon,
  TimelineIcon,
  UploadIcon,
  UserIcon,
  UsersIcon,
} from '../Icons/icons'
import { SETTINGS_PATH } from '../../paths'
import { gradient } from '../../Components/JobsPage/jobsTheme'
import { toneAccent, type PageTip, type TipIconName } from './pageTipData'

// Some of the app's icons come with a fixed size and no className, so the badge sizes whatever icon it holds
// itself (see the [&>svg] classes below).
const ICONS: Record<TipIconName, ComponentType> = {
  search: FilterIcon,
  match: MatchIcon,
  cv: PDFIcon,
  plus: PlusIcon,
  details: EyeIcon,
  job: BriefcaseIcon,
  company: BuildingIcon,
  people: UsersIcon,
  person: UserIcon,
  layers: LayersIcon,
  target: TargetIcon,
  timeline: TimelineIcon,
  camera: CameraIcon,
  upload: UploadIcon,
  monitor: PulseIcon,
  play: PlayIcon,
  stop: StopIcon,
  log: LogIcon,
  alert: AlertIcon,
  lock: LockIcon,
  spark: SparkIcon,
  robot: RobotIcon,
  bolt: BoltIcon,
  clock: ClockIcon,
  check: CheckCircleIcon,
  globe: GlobeIcon,
  link: LinkGlyphIcon,
  pencil: PencilIcon,
}

// One tip, laid out like a dashboard card's content: a gradient icon badge, then the title and the text. The card
// around it (tint, border, shapes) is the carousel's.
export function PageTipBody({ tip }: { tip: PageTip }) {
  const accent = toneAccent(tip.tone)
  const Icon = ICONS[tip.icon]
  return (
    <div className="flex items-start gap-4">
      <span
        className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm [&>svg]:h-5 [&>svg]:w-5"
        style={{ background: gradient(accent) }}
        aria-hidden="true"
      >
        <Icon />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-base font-semibold" style={{ color: accent.text }}>
          {tip.title}
        </p>
        <p className="mt-1 text-sm leading-6 text-gray-600">{tip.text}</p>
        {tip.settingsLink && (
          <a
            href={SETTINGS_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(SETTINGS_PATH)
            }}
            className="mt-2 inline-block text-sm font-medium hover:underline"
            style={{ color: accent.text }}
          >
            Open Settings →
          </a>
        )}
      </div>
    </div>
  )
}
