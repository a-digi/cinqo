import { AppLink } from '../../AppLink/AppLink'
import { LANDING, type LandingTone } from './landingTokens'

// The small pill button of the landing look: black on a tile ("dark"), white on the navy hero ("light"). A button that does something, or a real link
// to a page of the tool (`href`), with an optional arrow.
export function PillButton({
  children,
  tone = 'dark',
  href,
  onClick,
  arrow = false,
  ariaLabel,
}: {
  children: React.ReactNode
  tone?: LandingTone
  href?: string
  onClick?: () => void
  arrow?: boolean
  ariaLabel?: string
}) {
  const className = `inline-flex items-center gap-1.5 rounded-full px-5 py-2.5 text-sm font-bold transition-transform hover:-translate-y-0.5 motion-reduce:transition-none motion-reduce:hover:translate-y-0 ${
    tone === 'dark' ? 'text-white' : 'text-gray-900'
  }`
  const style = { background: tone === 'dark' ? LANDING.ink : '#ffffff' }
  const body = (
    <>
      {children}
      {arrow && <span aria-hidden="true">→</span>}
    </>
  )
  if (href !== undefined) {
    return (
      <AppLink href={href} className={className} style={style} aria-label={ariaLabel}>
        {body}
      </AppLink>
    )
  }
  return (
    <button type="button" onClick={onClick} className={className} style={style} aria-label={ariaLabel}>
      {body}
    </button>
  )
}
