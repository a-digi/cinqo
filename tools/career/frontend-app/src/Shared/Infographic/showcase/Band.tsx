import { Reveal } from '../Reveal'
import { toneColors, type ShowcaseTone } from './showcaseTokens'

// The words of a chapter of the showcase look: a small eyebrow, a very large heading, a lead, and what the chapter shows (`children`). The colours follow the band's tone, so a
// chapter reads the same on black and on light grey. The band's own full-width background is drawn by the modal around it.
export function Band({
  tone,
  eyebrow,
  title,
  titleId,
  lead,
  children,
  align = 'center',
}: {
  tone: ShowcaseTone
  eyebrow?: string
  title: string
  titleId?: string
  lead?: string
  children?: React.ReactNode
  align?: 'center' | 'left'
}) {
  const c = toneColors(tone)
  return (
    <div className={`mx-auto max-w-6xl ${align === 'center' ? 'text-center' : 'text-left'}`} data-tone={tone}>
      <Reveal>
        {eyebrow !== undefined && eyebrow !== '' && (
          <p data-testid="band-eyebrow" className="mb-3 text-xl font-semibold md:text-2xl" style={{ color: c.muted }}>
            {eyebrow}
          </p>
        )}
        <h3 id={titleId} className="text-5xl font-semibold leading-[1.05] tracking-[-0.025em] md:text-7xl" style={{ color: c.color }}>
          {title}
        </h3>
        {lead !== undefined && lead !== '' && (
          <p
            className={`mt-6 max-w-3xl text-xl leading-snug md:text-2xl ${align === 'center' ? 'mx-auto' : ''}`}
            style={{ color: c.muted }}
          >
            {lead}
          </p>
        )}
      </Reveal>
      {children !== undefined && <div className="mt-14">{children}</div>}
    </div>
  )
}
