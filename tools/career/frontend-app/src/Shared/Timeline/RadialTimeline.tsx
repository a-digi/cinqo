import { useEffect, useId, useMemo, useState } from 'react'
import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { AppLink } from '../AppLink/AppLink'
import { StepIcon } from './StepIcon'
import { StepBadge, StepWords } from './StepParts'
import { RADIAL, bandPath, fitFontSize, labelPoint, panelPath, textRects, wordsLength } from './radialRules'
import { nodeLook, numberLabel, type TimelineStep } from './timelineRules'

const ENTER_DELAY_MS = 110

// The radial timeline (plan/ai/tools/career/step-170-onboarding-guide.md), after a ring infographic: the ring is split into one sector per
// step, each with a coloured band on the outside that carries the step's number, a white wedge panel inside it with the step's picture,
// title, words and link, and a hub in the middle with the title and a colour legend. The sectors sweep in one after the other. The wedges
// are drawn in a 1000-unit box that scales with the page, and the words sit in the biggest upright rectangle that fits each wedge, at a
// font size fitted to how much there is to say (see radialRules.ts); that is why it needs room and is drawn from the xl breakpoint up.
// Below that the steps come as one column of cards. Exactly one of the two lists is on screen at a time.
export function RadialTimeline({
  steps,
  ariaLabel,
  accentOf,
}: {
  steps: readonly TimelineStep[]
  ariaLabel: string
  accentOf: (index: number) => Accent
}) {
  const count = steps.length
  const uid = useId().replace(/[^a-zA-Z0-9]/g, '')
  const rects = useMemo(() => textRects(count), [count])
  const [ready, setReady] = useState(false)
  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      setReady(true)
    })
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [])
  const hubWidth = ((RADIAL.hub * 2 * 0.8) / RADIAL.size) * 100
  return (
    <>
      <div className="relative mx-auto hidden aspect-square w-full max-w-[1000px] [container-type:inline-size] xl:block">
        {/* The faint dot pattern behind the ring, like the map in the template. */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0"
          style={{
            backgroundImage: 'radial-gradient(circle, rgba(148,163,184,0.4) 1.6px, transparent 2px)',
            backgroundSize: '22px 22px',
            maskImage: 'radial-gradient(circle at center, #000 48%, transparent 70%)',
            WebkitMaskImage: 'radial-gradient(circle at center, #000 48%, transparent 70%)',
          }}
        />
        <svg
          viewBox={`0 0 ${String(RADIAL.size)} ${String(RADIAL.size)}`}
          className="absolute inset-0 h-full w-full overflow-visible"
          aria-hidden="true"
        >
          <defs>
            <filter id={`${uid}-shadow`} x="-20%" y="-20%" width="140%" height="140%">
              <feDropShadow dx="0" dy="5" stdDeviation="7" floodColor="#0f172a" floodOpacity="0.16" />
            </filter>
            {steps.map((step, index) => {
              const accent = accentOf(index)
              return (
                <linearGradient key={`gradient-${step.id}`} id={`${uid}-band-${String(index)}`} x1="0" y1="0" x2="1" y2="1">
                  <stop offset="0" stopColor={accent.from} />
                  <stop offset="1" stopColor={accent.to} />
                </linearGradient>
              )
            })}
          </defs>
          {steps.map((step, index) => {
            const label = labelPoint(index, count)
            return (
              <g
                key={step.id}
                className="transition-[transform,opacity] duration-700 ease-out motion-reduce:transition-none"
                style={{
                  transformOrigin: `${String(RADIAL.center)}px ${String(RADIAL.center)}px`,
                  transform: ready ? 'rotate(0deg) scale(1)' : 'rotate(-28deg) scale(0.88)',
                  opacity: ready ? 1 : 0,
                  transitionDelay: `${String(index * ENTER_DELAY_MS)}ms`,
                }}
              >
                <path d={panelPath(index, count)} fill="#ffffff" filter={`url(#${uid}-shadow)`} />
                <path d={bandPath(index, count)} fill={`url(#${uid}-band-${String(index)})`} filter={`url(#${uid}-shadow)`} />
                <text x={label.x} y={label.y} textAnchor="middle" dominantBaseline="central" fill="#ffffff" fontSize="26" fontWeight="800">
                  {nodeLook(step) === 'done' ? '✓' : numberLabel(index)}
                </text>
              </g>
            )
          })}
          <circle cx={RADIAL.center} cy={RADIAL.center} r={RADIAL.hub} fill="#ffffff" filter={`url(#${uid}-shadow)`} />
        </svg>
        {/* The hub: the title, how many steps and one colour segment per step. */}
        <div
          className="absolute left-1/2 top-1/2 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center text-center"
          style={{ width: `${String(hubWidth)}%` }}
          aria-hidden="true"
        >
          <p className="font-extrabold leading-tight text-gray-800" style={{ fontSize: '2.3cqw' }}>
            {ariaLabel}
          </p>
          <p className="mt-[0.5cqw] font-semibold uppercase tracking-widest text-gray-400" style={{ fontSize: '1.05cqw' }}>
            {`${String(count)} steps`}
          </p>
          <div className="mt-[1.1cqw] flex h-[0.8cqw] w-[11cqw] overflow-hidden rounded-full">
            {steps.map((step, index) => (
              <span key={`legend-${step.id}`} className="flex-1" style={{ background: accentOf(index).from }} />
            ))}
          </div>
        </div>
        <ol aria-label={ariaLabel} className="absolute inset-0 m-0 list-none p-0">
          {steps.map((step, index) => {
            const accent = accentOf(index)
            const rect = rects[index]
            const size = fitFontSize(wordsLength(step), rect.w, rect.h)
            return (
              <li
                key={step.id}
                aria-current={nodeLook(step) === 'current' ? 'step' : undefined}
                className="absolute flex flex-col justify-center transition-opacity duration-700 motion-reduce:transition-none"
                style={{
                  left: `${String(rect.x / 10)}%`,
                  top: `${String(rect.y / 10)}%`,
                  width: `${String(rect.w / 10)}%`,
                  height: `${String(rect.h / 10)}%`,
                  fontSize: `${String(size / 10)}cqw`,
                  opacity: ready ? 1 : 0,
                  transitionDelay: `${String(index * ENTER_DELAY_MS + 250)}ms`,
                }}
              >
                {step.icon !== undefined && (
                  <span className="mb-[0.4em]" style={{ color: accent.to, fontSize: '1.25em' }} aria-hidden="true">
                    <StepIcon name={step.icon} className="text-inherit" />
                  </span>
                )}
                <h2 className="font-extrabold leading-tight" style={{ color: accent.text, fontSize: '1.15em' }}>
                  <span className="sr-only">{`Step ${numberLabel(index)}: `}</span>
                  {step.title}
                </h2>
                <p className="mt-[0.3em] leading-[1.35] text-gray-600">{step.text}</p>
                {step.tip !== undefined && (
                  <p
                    className="mt-[0.45em] rounded-[0.6em] px-[0.6em] py-[0.35em] leading-[1.3]"
                    style={{ background: accent.soft, color: accent.text, fontSize: '0.92em' }}
                  >
                    <span className="font-semibold">Tip: </span>
                    {step.tip}
                  </p>
                )}
                {step.link !== undefined && (
                  <AppLink
                    href={step.link.path}
                    className="mt-[0.45em] inline-block font-semibold hover:underline"
                    style={{ color: accent.text, fontSize: '0.95em' }}
                  >
                    {step.link.label} →
                  </AppLink>
                )}
              </li>
            )
          })}
        </ol>
      </div>
      <ol aria-label={ariaLabel} className="m-0 list-none space-y-3 p-0 xl:hidden">
        {steps.map((step, index) => {
          const accent = accentOf(index)
          return (
            <li
              key={step.id}
              aria-current={nodeLook(step) === 'current' ? 'step' : undefined}
              className="relative overflow-hidden rounded-2xl border p-4 pl-5 shadow-sm"
              style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 45%, ${accent.soft} 100%)` }}
            >
              <span aria-hidden="true" className="absolute inset-y-0 left-0 w-1.5" style={{ background: gradient(accent) }} />
              <div className="flex items-start gap-3">
                <StepBadge step={step} index={index} accent={accent} />
                <div className="min-w-0 flex-1 pt-1">
                  <StepWords step={step} index={index} accent={accent} />
                </div>
              </div>
            </li>
          )
        })}
      </ol>
    </>
  )
}
