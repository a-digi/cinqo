import { AlertIcon, LockIcon, RobotIcon, SparkIcon } from '../../Shared/Icons/icons'
import { SETTINGS_PATH } from '../../paths'
import { ACCENTS } from '../JobsPage/jobsTheme'
import type { PortalTipData, PortalTipTone } from './portalTipData'

const TONE_ACCENT: Record<PortalTipTone, (typeof ACCENTS)[number]> = {
  amber: ACCENTS[2],
  rose: ACCENTS[3],
  green: ACCENTS[1],
  violet: ACCENTS[4],
}

function ToneIcon({ tone }: { tone: PortalTipTone }) {
  switch (tone) {
    case 'amber':
      return <AlertIcon />
    case 'rose':
      return <LockIcon className="h-4 w-4" />
    case 'green':
      return <SparkIcon className="h-4 w-4" />
    case 'violet':
      return <RobotIcon className="h-4 w-4" />
  }
}

// One tip: a coloured icon tile, a short title and the text. Flat, like the column it sits in.
export function PortalTip({ tip }: { tip: PortalTipData }) {
  const accent = TONE_ACCENT[tip.tone]
  return (
    <div className="pr-1">
      <span
        className="mb-2 inline-flex h-7 w-7 items-center justify-center rounded-lg"
        style={{ background: accent.soft, color: accent.text }}
        aria-hidden="true"
      >
        <ToneIcon tone={tip.tone} />
      </span>
      <p className="text-sm font-semibold text-gray-900">{tip.title}</p>
      <p className="mt-1 text-xs leading-5 text-gray-600">{tip.text}</p>
      {tip.settingsLink && (
        <a
          href={SETTINGS_PATH}
          onClick={(e) => {
            e.preventDefault()
            window.__cinqoToolBridge.navigate(SETTINGS_PATH)
          }}
          className="mt-2 inline-block text-xs font-medium text-blue-700 hover:underline"
        >
          Open Settings →
        </a>
      )}
    </div>
  )
}
