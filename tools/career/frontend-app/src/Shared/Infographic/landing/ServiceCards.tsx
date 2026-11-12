import { LANDING } from './landingTokens'

export interface ServiceCard {
  id: string
  title: string
  text: string
  action: React.ReactNode
}

// Two (or more) centred cards at the end of the page, each with a heading, a line of text and a pill button (the landing look's service cards).
export function ServiceCards({ cards }: { cards: ServiceCard[] }) {
  return (
    <ul className="mx-auto grid max-w-4xl gap-10 text-center md:grid-cols-2" data-testid="service-cards">
      {cards.map((c) => (
        <li key={c.id}>
          <h4 className="text-lg font-extrabold" style={{ color: LANDING.ink }}>
            {c.title}
          </h4>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed" style={{ color: LANDING.muted }}>
            {c.text}
          </p>
          <div className="mt-4 flex justify-center">{c.action}</div>
        </li>
      ))}
    </ul>
  )
}
