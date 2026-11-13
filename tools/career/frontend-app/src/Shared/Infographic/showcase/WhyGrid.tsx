import { Reveal } from '../Reveal'

export interface WhyCard {
  id: string
  icon: React.ReactNode
  title: string
  text: string
  // The number of the footnote this card refers to (it is drawn as a small raised number), or none.
  note?: number
}

// The closing grid of cards (the showcase look's "why" block): up to six cards, an icon, a bold heading and a short text each, rounded and light grey, three to a row on a wide window.
export function WhyGrid({ cards }: { cards: WhyCard[] }) {
  return (
    <ul className="mx-auto grid max-w-6xl gap-5 md:grid-cols-2 lg:grid-cols-3" data-testid="why-grid">
      {cards.map((c, i) => (
        <Reveal as="li" key={c.id} delay={i % 3}>
          <div className="h-full rounded-[1.75rem] bg-white p-8 text-left">
            <div className="mb-5 h-8 w-8 text-[#1d1d1f]" aria-hidden="true">
              {c.icon}
            </div>
            <h4 className="text-xl font-semibold leading-snug text-[#1d1d1f]">
              {c.title}
              {c.note !== undefined && (
                <sup data-testid="why-note" className="ml-0.5 text-xs font-normal text-[#6e6e73]">
                  <a href={`#showcase-note-${String(c.note)}`} aria-label={`Note ${String(c.note)}`}>
                    {c.note}
                  </a>
                </sup>
              )}
            </h4>
            <p className="mt-3 text-base leading-relaxed text-[#6e6e73]">{c.text}</p>
          </div>
        </Reveal>
      ))}
    </ul>
  )
}
