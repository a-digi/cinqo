import { Reveal } from '../Reveal'
import { toneColors, type ShowcaseTone } from './showcaseTokens'

export interface IconRowItem {
  id: string
  icon: React.ReactNode
  title: string
  text: string
}

// A row of four (or so) things, each a line icon, a short heading and one line (the showcase look's spec row). Two to a row on a narrow window, all in one row on a wide one.
export function IconRow({ items, tone }: { items: IconRowItem[]; tone: ShowcaseTone }) {
  const c = toneColors(tone)
  return (
    <ul className="mx-auto grid max-w-6xl grid-cols-2 gap-x-6 gap-y-12 md:grid-cols-4" data-testid="icon-row">
      {items.map((it, i) => (
        <Reveal as="li" key={it.id} delay={i}>
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center" style={{ color: c.color }} aria-hidden="true">
            {it.icon}
          </div>
          <h4 className="text-xl font-semibold" style={{ color: c.color }}>
            {it.title}
          </h4>
          <p className="mx-auto mt-2 max-w-[16rem] text-base leading-snug" style={{ color: c.muted }}>
            {it.text}
          </p>
        </Reveal>
      ))}
    </ul>
  )
}
