import { AlertIcon, LockIcon, RobotIcon, SparkIcon } from '../../Shared/Icons/icons'
import { SETTINGS_PATH } from '../../paths'
import { gradient } from '../JobsPage/jobsTheme'
import { toneAccent, type PortalTipData, type PortalTipTone } from './portalTipData'

const BADGE_ICON = 'h-5 w-5'

function ToneIcon({ tone }: { tone: PortalTipTone }) {
  switch (tone) {
    case 'amber':
      return <AlertIcon className={BADGE_ICON} />
    case 'rose':
      return <LockIcon className={BADGE_ICON} />
    case 'green':
      return <SparkIcon className={BADGE_ICON} />
    case 'violet':
      return <RobotIcon className={BADGE_ICON} />
  }
}

// One tip, laid out like a dashboard card's content: a gradient icon badge, then the title and the text. The
// card around it (tint, border, shapes) is the carousel's.
export function PortalTip({ tip }: { tip: PortalTipData }) {
  const accent = toneAccent(tip.tone)
  return (
    <div className="flex items-start gap-4">
      <span
        className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm"
        style={{ background: gradient(accent) }}
        aria-hidden="true"
      >
        <ToneIcon tone={tip.tone} />
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
