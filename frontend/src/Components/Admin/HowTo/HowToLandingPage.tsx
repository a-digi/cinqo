import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Eyebrow, Frame, In, Subtitle, Title, type Motif } from '../Platforms/PlatformSlides'
import type { HowToPageContent } from './HowToContent'
import { howToPath, neighbours } from './howToRules'

export interface HowToHero {
  from: string
  to: string
  motif: Motif
  // Badges next to the eyebrow (free / premium, on or off).
  badges?: ReactNode
  // The buttons under the promise.
  actions?: ReactNode
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl bg-white p-5 ring-1 ring-gray-200 md:p-6">
      <h2 className="text-base font-semibold text-gray-900">{title}</h2>
      <div className="mt-3">{children}</div>
    </section>
  )
}

function Bullets({ items, tone }: { items: string[]; tone: 'good' | 'careful' }) {
  return (
    <ul className="space-y-2">
      {items.map((t) => (
        <li key={t} className="flex gap-2 text-sm leading-relaxed text-gray-700">
          <span
            aria-hidden="true"
            className={`mt-1 h-2 w-2 shrink-0 rounded-full ${tone === 'good' ? 'bg-emerald-500' : 'bg-amber-500'}`}
          />
          {t}
        </li>
      ))}
    </ul>
  )
}

// One landing page of the How To section (plan 219): a hero in the look of the slides, then the same sections on every page, then the way on. It
// shows what it is given: the words come from HowToContent, the live parts (models, state, buttons) are passed in by the route.
export function HowToLandingPage({
  content,
  hero,
  models,
  modelsTitle = 'The models it offers',
}: {
  content: HowToPageContent
  hero: HowToHero
  // The live list of models of this page, when it has one.
  models?: ReactNode
  modelsTitle?: string
}) {
  const { previous, next } = neighbours(content.id)
  return (
    <div className="max-w-6xl space-y-6 p-6">
      <div className="overflow-hidden rounded-3xl shadow-xl">
        <Frame from={hero.from} to={hero.to} motif={hero.motif}>
          <div className="max-w-2xl pb-6">
            <In i={0} className="flex flex-wrap items-center gap-2">
              <Eyebrow>{content.eyebrow}</Eyebrow>
              {hero.badges}
            </In>
            <In i={1}>
              <Title>{content.title}</Title>
            </In>
            <In i={2}>
              <Subtitle>{content.promise}</Subtitle>
            </In>
            {hero.actions && (
              <In i={3} className="mt-6 flex flex-wrap items-center gap-3">
                {hero.actions}
              </In>
            )}
          </div>
        </Frame>
      </div>

      <section aria-label="At a glance" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {content.glance.map((t) => (
          <div key={t.title} className="rounded-2xl bg-gray-50 p-4 ring-1 ring-gray-200">
            <p className="text-sm font-semibold text-gray-900">{t.title}</p>
            <p className="mt-1 text-sm leading-relaxed text-gray-600">{t.text}</p>
          </div>
        ))}
      </section>

      <div className="grid gap-6 md:grid-cols-2">
        <Section title="Good for">
          <Bullets items={content.goodFor} tone="good" />
        </Section>
        <Section title="Not so good for">
          <Bullets items={content.notSoGood} tone="careful" />
        </Section>
      </div>

      <Section title={`Get started in ${content.steps.length} steps`}>
        <ol className="space-y-4">
          {content.steps.map((s, i) => (
            <li key={s.title} className="flex gap-3">
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gray-900 text-sm font-semibold text-white">
                {i + 1}
              </span>
              <div>
                <p className="text-sm font-semibold text-gray-900">{s.title}</p>
                <p className="text-sm leading-relaxed text-gray-600">{s.text}</p>
              </div>
            </li>
          ))}
        </ol>
      </Section>

      {models && <Section title={modelsTitle}>{models}</Section>}

      <div className="grid gap-6 md:grid-cols-2">
        <Section title="Cost and limits">
          <p className="text-sm leading-relaxed text-gray-700">{content.cost}</p>
        </Section>
        <Section title="Your data and your key">
          <p className="text-sm leading-relaxed text-gray-700">{content.data}</p>
        </Section>
      </div>

      <Section title="Questions people ask">
        <div className="divide-y divide-gray-100">
          {content.questions.map((q) => (
            <details key={q.question} className="group py-3 first:pt-0 last:pb-0">
              <summary className="cursor-pointer list-none text-sm font-medium text-gray-900 marker:hidden">
                <span className="mr-2 inline-block text-gray-400 transition-transform group-open:rotate-90" aria-hidden="true">
                  ›
                </span>
                {q.question}
              </summary>
              <p className="mt-2 pl-4 text-sm leading-relaxed text-gray-600">{q.answer}</p>
            </details>
          ))}
        </div>
      </Section>

      <nav aria-label="More how-to pages" className="flex items-center justify-between gap-3 pt-2">
        {previous ? (
          <Link
            to={howToPath(previous)}
            className="rounded-full px-4 py-2 text-sm font-medium text-gray-700 ring-1 ring-gray-300 hover:bg-gray-50"
          >
            ← Previous
          </Link>
        ) : (
          <span />
        )}
        {next ? (
          <Link to={howToPath(next)} className="rounded-full bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800">
            Next →
          </Link>
        ) : (
          <span />
        )}
      </nav>
    </div>
  )
}
