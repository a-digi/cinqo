import { AppLink } from '../../AppLink/AppLink'
import { SHOWCASE } from './showcaseTokens'

export type ShowcasePillTone = 'blue' | 'outline-dark' | 'outline-light'

// The pill of the showcase look: filled blue (the one call to action of a band), or an outlined one on a dark or a light band. A button, or a real link to a page of the tool
// (`href`), with an optional arrow.
export function ShowcasePill({
  children,
  tone = 'blue',
  href,
  onClick,
  arrow = false,
  ariaLabel,
}: {
  children: React.ReactNode
  tone?: ShowcasePillTone
  href?: string
  onClick?: () => void
  arrow?: boolean
  ariaLabel?: string
}) {
  const className = `inline-flex items-center gap-1.5 rounded-full px-6 py-3 text-base font-medium transition-colors motion-reduce:transition-none ${
    tone === 'blue'
      ? 'text-white hover:brightness-110'
      : tone === 'outline-dark'
        ? 'border border-white/60 text-white hover:bg-white/10'
        : 'border border-[#1d1d1f]/40 text-[#1d1d1f] hover:bg-black/5'
  }`
  const style = tone === 'blue' ? { background: SHOWCASE.blue } : undefined
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
