import { Reveal } from '../Reveal'

export interface CompareTile {
  id: string
  title: string
  tagline: string
  points: string[]
  action?: React.ReactNode
}

// Two (or more) tiles side by side that set things against each other (the showcase look's product tiles): a heading, a tagline and a short list. On a light band the tiles are
// white, on a dark one a dark grey; a narrow window puts them one under the other.
export function CompareTiles({ tiles, dark = false }: { tiles: CompareTile[]; dark?: boolean }) {
  return (
    <ul className="mx-auto grid max-w-5xl gap-6 md:grid-cols-2" data-testid="compare-tiles">
      {tiles.map((t, i) => (
        <Reveal as="li" key={t.id} delay={i}>
          <div
            className="flex h-full flex-col rounded-[2rem] p-8 text-left md:p-10"
            style={{ background: dark ? '#1d1d1f' : '#ffffff', color: dark ? '#ffffff' : '#1d1d1f' }}
          >
            <h4 className="text-3xl font-semibold tracking-tight">{t.title}</h4>
            <p className="mt-2 text-lg" style={{ color: dark ? '#a1a1a6' : '#6e6e73' }}>
              {t.tagline}
            </p>
            <ul className="mt-6 space-y-3 text-base leading-snug">
              {t.points.map((p) => (
                <li key={p} className="flex gap-3">
                  <span aria-hidden="true" className="mt-1 text-sm" style={{ color: '#0071e3' }}>
                    ✓
                  </span>
                  <span>{p}</span>
                </li>
              ))}
            </ul>
            {t.action !== undefined && <div className="mt-8">{t.action}</div>}
          </div>
        </Reveal>
      ))}
    </ul>
  )
}
