import { gradient, type Accent } from '../../Components/JobsPage/jobsTheme'
import { StepIcon } from './StepIcon'
import { StepWords } from './StepParts'
import { edgeMask } from './scrollRules'
import {
  PIN_FIRST_RING_REM,
  PIN_INTERVAL,
  blockColumn,
  isAbove,
  isLast,
  nodeLook,
  numberLabel,
  ringLeft,
  trackColumns,
  trackWidth,
} from './timelineRules'
import type { TimelineStep } from './timelineRules'
import { useHorizontalScroll } from './useHorizontalScroll'
import { useReveal } from './useReveal'

// One teardrop pin: a rounded square turned 45 degrees so its sharp corner points at the bar (down for a pin above it, up for a pin below
// it), with the step's picture (or its number) in white on top. The box is a little taller than the round part so the tip fits in it.
function Pin({ step, index, accent, tipDown }: { step: TimelineStep; index: number; accent: Accent; tipDown: boolean }) {
  const look = nodeLook(step)
  const place = tipDown ? 'top-0' : 'bottom-0'
  return (
    <span className="relative block h-14 w-12 shrink-0" aria-hidden="true">
      <span
        className={`absolute left-0 h-12 w-12 rotate-45 shadow-sm ${place} ${tipDown ? 'rounded-[50%_50%_0_50%]' : 'rounded-[0_50%_50%_50%]'}`}
        style={{ background: gradient(accent) }}
      />
      <span className={`absolute left-0 flex h-12 w-12 items-center justify-center text-base font-bold text-white ${place}`}>
        {look === 'done' ? '✓' : step.icon !== undefined ? <StepIcon name={step.icon} /> : numberLabel(index)}
      </span>
    </span>
  )
}

function Chevron({ left }: { left: boolean }) {
  return (
    <svg
      viewBox="0 0 20 20"
      className="h-5 w-5"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={left ? 'M12.5 4 L6.5 10 L12.5 16' : 'M7.5 4 L13.5 10 L7.5 16'} />
    </svg>
  )
}

const ARROW_REM = 3
const BUTTON =
  'absolute top-1/2 z-10 inline-flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-700 shadow-md transition-opacity hover:bg-gray-50'

// The horizontal timeline (plan/ai/tools/career/step-170-onboarding-guide.md): a thick bar made of one segment per step in that step's
// colour, a ring on the bar for each step with its number, a big arrow at the end, and a pin with the step's text beside it above or below
// the ring in turn, like a project roadmap. It scrolls sideways, so every step has room for its words (the interval between two steps is
// --tl-i, smaller on a phone). While it scrolls: the edge that has more behind it fades out, round buttons and a progress thumb show where
// you are, a mouse can drag it, and each step (pin, ring, the bar's segment) comes in as it scrolls into view.
export function PinsTimeline({
  steps,
  ariaLabel,
  accentOf,
}: {
  steps: readonly TimelineStep[]
  ariaLabel: string
  accentOf: (index: number) => Accent
}) {
  const count = steps.length
  const scroll = useHorizontalScroll()
  const seen = useReveal(
    scroll.ref,
    steps.map((step) => step.id),
  )
  const { edges, progress, thumb } = scroll.state
  const scrollable = edges.canLeft || edges.canRight
  const mask = edgeMask(edges)
  const barEnd = `calc(${PIN_INTERVAL} * ${String(Math.max(count - 1, 0))} + ${String(PIN_FIRST_RING_REM)}rem + ${PIN_INTERVAL} * 0.9)`
  return (
    <div className="relative">
      <div
        ref={scroll.ref}
        role="region"
        aria-label={`${ariaLabel}, scrolls sideways`}
        // Deliberate: a sideways scrolling area must be reachable by keyboard, or its arrow keys cannot scroll it.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={0}
        className={`overflow-x-auto overscroll-x-contain rounded-3xl pb-3 outline-none [scrollbar-width:none] focus-visible:ring-2 focus-visible:ring-blue-300 [&::-webkit-scrollbar]:hidden ${
          scrollable ? (scroll.dragging ? 'cursor-grabbing select-none' : 'cursor-grab') : ''
        }`}
        style={{ maskImage: mask, WebkitMaskImage: mask }}
        {...scroll.dragProps}
      >
        <ol
          aria-label={ariaLabel}
          className="m-0 grid list-none p-0 [--tl-i:9rem] sm:[--tl-i:12.5rem]"
          style={{ width: trackWidth(count), gridTemplateColumns: trackColumns(count), gridTemplateRows: 'auto 3.25rem auto' }}
        >
          {/* The bar: decorative, the steps carry the words. Each segment is drawn from its ring as that step comes into view. */}
          <li aria-hidden="true" className="relative" style={{ gridRow: 2, gridColumn: '1 / -1' }}>
            {steps.map((step, index) => {
              const accent = accentOf(index)
              const last = isLast(index, count)
              return (
                <span
                  key={`segment-${step.id}`}
                  className={`absolute top-1/2 h-4 origin-left -translate-y-1/2 transition-transform duration-700 ease-out motion-reduce:transition-none ${index === 0 ? 'rounded-l-full' : ''} ${
                    seen.has(step.id) ? 'scale-x-100' : 'scale-x-0'
                  }`}
                  style={{
                    left: ringLeft(index),
                    width: last ? `calc(${PIN_INTERVAL} * 0.9 - 1.4rem)` : PIN_INTERVAL,
                    background: accent.from,
                  }}
                />
              )
            })}
            {count > 0 && (
              <svg
                viewBox="0 0 56 64"
                className={`absolute top-1/2 h-14 w-12 -translate-y-1/2 transition-opacity duration-700 motion-reduce:transition-none ${
                  seen.has(steps[count - 1].id) ? 'opacity-100' : 'opacity-0'
                }`}
                style={{ left: `calc(${barEnd} - ${String(ARROW_REM)}rem)` }}
                fill="none"
                stroke={accentOf(count - 1).from}
                strokeWidth="15"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M12 8 L46 32 L12 56" />
              </svg>
            )}
            {steps.map((step, index) => {
              const accent = accentOf(index)
              return (
                <span
                  key={`ring-${step.id}`}
                  className={`absolute top-1/2 flex h-11 w-11 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border-[7px] bg-white text-[11px] font-bold shadow-sm transition duration-500 ease-out motion-reduce:transition-none ${
                    seen.has(step.id) ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
                  }`}
                  style={{ left: ringLeft(index), borderColor: accent.to, color: accent.to }}
                >
                  {nodeLook(step) === 'done' ? '✓' : numberLabel(index)}
                </span>
              )
            })}
          </li>
          {steps.map((step, index) => {
            const accent = accentOf(index)
            const above = isAbove(index)
            const shown = seen.has(step.id)
            return (
              <li
                key={step.id}
                data-step-id={step.id}
                aria-current={nodeLook(step) === 'current' ? 'step' : undefined}
                className={`flex gap-3 pr-4 transition duration-500 ease-out motion-reduce:translate-y-0 motion-reduce:opacity-100 motion-reduce:transition-none ${
                  above ? 'items-end pb-1' : 'items-start pt-1'
                } ${shown ? 'translate-y-0 opacity-100' : above ? 'translate-y-5 opacity-0' : '-translate-y-5 opacity-0'}`}
                style={{ gridColumn: `${String(blockColumn(index))} / span 2`, gridRow: above ? 1 : 3 }}
              >
                <Pin step={step} index={index} accent={accent} tipDown={above} />
                <div className={`min-w-0 flex-1 ${above ? 'pb-2' : 'pt-2'}`}>
                  <StepWords step={step} index={index} accent={accent} compact />
                </div>
              </li>
            )
          })}
        </ol>
      </div>
      {scrollable && (
        <>
          <button
            type="button"
            aria-label="Scroll the timeline to the left"
            onClick={() => {
              scroll.page(-1)
            }}
            className={`${BUTTON} left-1 ${edges.canLeft ? 'opacity-100' : 'pointer-events-none opacity-0'}`}
          >
            <Chevron left />
          </button>
          <button
            type="button"
            aria-label="Scroll the timeline to the right"
            onClick={() => {
              scroll.page(1)
            }}
            className={`${BUTTON} right-1 ${edges.canRight ? 'opacity-100' : 'pointer-events-none opacity-0'}`}
          >
            <Chevron left={false} />
          </button>
          <div className="mt-1 flex items-center justify-center gap-3" aria-hidden="true">
            <div className="h-1.5 w-40 rounded-full bg-gray-200">
              <div
                className="h-full rounded-full bg-blue-500 transition-[margin-left] duration-150"
                style={{ width: `${String(thumb * 100)}%`, marginLeft: `${String((1 - thumb) * progress * 100)}%` }}
              />
            </div>
            <span className="text-xs text-gray-400">Scroll or drag sideways</span>
          </div>
        </>
      )}
    </div>
  )
}
