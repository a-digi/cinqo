import { useCallback, useEffect, useId, useState, type ReactNode } from 'react'
import { autoPlayInterval, clampIndex, slideLabel, stepIndex, type Direction } from './sliderRules'
import { useSwipe } from './useSwipe'

export interface SliderSlide {
  // Stable key for the slide.
  id: string
  // A short name: the dot's tooltip and what a screen reader hears ("2 of 4: OpenAI").
  label: string
  content: ReactNode
}

export interface SliderProps {
  slides: SliderSlide[]
  // The frame's accessible name ("About AI platforms").
  ariaLabel: string
  // Controlled use: the page owns the position. Leave both out and the slider keeps its own.
  index?: number
  onIndexChange?: (index: number) => void
  // Wrap from the last slide to the first (and back).
  loop?: boolean
  // Milliseconds between automatic slides; 0 (default) means it never moves on its own. It pauses while
  // the pointer or keyboard focus is inside, and never runs under prefers-reduced-motion.
  autoPlayMs?: number
  showArrows?: boolean
  showDots?: boolean
  // 'dark' is for slides with a dark, full-bleed backdrop: glass arrows and a progress bar in a bottom row
  // with a 01 / 04 counter. 'light' (default) is the plain white-surface look.
  tone?: 'light' | 'dark'
  className?: string
}

const SLIDE_MS = 600

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches)
  useEffect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => {
      setReduced(query.matches)
    }
    query.addEventListener('change', update)
    return () => {
      query.removeEventListener('change', update)
    }
  }, [])
  return reduced
}

function Chevron({ direction }: { direction: Direction }) {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4" aria-hidden="true">
      <path d={direction === 'next' ? 'M7.5 4.5 13 10l-5.5 5.5' : 'M12.5 4.5 7 10l5.5 5.5'} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// A shared slider (carousel): one slide at a time, arrows, dots, the left and right arrow keys and
// swipe. Purely presentational: it takes slides and shows them, no data, no fetching. Every slide is
// rendered (the frame is as tall as the tallest, so nothing jumps), the ones not shown are aria-hidden
// and inert so focus and screen readers skip them. The track moves slowly (600 ms) and not at all
// under prefers-reduced-motion.
//
//   <Slider ariaLabel="About AI platforms" slides={[{ id: 'intro', label: 'Overview', content: <Intro /> }]} />
//
// Use index + onIndexChange to control it from the page (for example to jump to a slide).
export function Slider({
  slides,
  ariaLabel,
  index: controlledIndex,
  onIndexChange,
  loop = false,
  autoPlayMs = 0,
  showArrows = true,
  showDots = true,
  tone = 'light',
  className = '',
}: SliderProps) {
  const count = slides.length
  const [ownIndex, setOwnIndex] = useState(0)
  const index = clampIndex(controlledIndex ?? ownIndex, count)
  const [paused, setPaused] = useState(false)
  // Said to screen readers after the user (not the timer) moved the slider.
  const [announcement, setAnnouncement] = useState('')
  const reducedMotion = useReducedMotion()
  const regionId = useId()

  const goTo = useCallback(
    (target: number, byUser: boolean) => {
      const next = clampIndex(target, count)
      setOwnIndex(next)
      onIndexChange?.(next)
      setAnnouncement(byUser ? slideLabel(next, count, slides[next]?.label) : '')
    },
    [count, onIndexChange, slides],
  )

  const step = (direction: Direction) => {
    goTo(stepIndex(index, count, direction, loop), true)
  }

  const swipe = useSwipe(step)

  const interval = autoPlayInterval(autoPlayMs, count, reducedMotion, paused)
  useEffect(() => {
    if (interval === 0) return
    const timer = window.setInterval(() => {
      const next = stepIndex(index, count, 'next', loop)
      if (next !== index) goTo(next, false)
    }, interval)
    return () => {
      window.clearInterval(timer)
    }
  }, [interval, index, count, loop, goTo])

  if (count === 0) return null
  const atStart = !loop && index === 0
  const atEnd = !loop && index === count - 1
  const arrowClass =
    'absolute top-1/2 z-10 flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-white/90 text-gray-700 shadow-md ring-1 ring-gray-200 transition hover:bg-white hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:pointer-events-none disabled:opacity-0'

  const glassArrow =
    'flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white ring-1 ring-white/30 backdrop-blur transition hover:bg-white/25 focus:outline-none focus-visible:ring-2 focus-visible:ring-white disabled:pointer-events-none disabled:opacity-30'

  return (
    // The frame is focusable and handles the arrow keys on purpose: that is the keyboard behaviour of the
    // WAI-ARIA carousel pattern (a region that moves with Left and Right), which this rule cannot express.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <section
      aria-roledescription="carousel"
      aria-label={ariaLabel}
      className={`relative overflow-hidden outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${className}`}
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'ArrowLeft') {
          e.preventDefault()
          step('prev')
        } else if (e.key === 'ArrowRight') {
          e.preventDefault()
          step('next')
        }
      }}
      onMouseEnter={() => {
        setPaused(true)
      }}
      onMouseLeave={() => {
        setPaused(false)
      }}
      onFocus={() => {
        setPaused(true)
      }}
      onBlur={() => {
        setPaused(false)
      }}
    >
      <div
        id={regionId}
        className="flex"
        style={{
          transform: `translateX(-${index * 100}%)`,
          transition: reducedMotion ? 'none' : `transform ${SLIDE_MS}ms cubic-bezier(0.22, 0.61, 0.36, 1)`,
          touchAction: 'pan-y',
        }}
        {...swipe}
      >
        {slides.map((slide, i) => (
          <div
            key={slide.id}
            role="group"
            aria-roledescription="slide"
            aria-label={slideLabel(i, count, slide.label)}
            aria-hidden={i !== index}
            inert={i !== index}
            data-slide-active={i === index}
            className="min-w-0 shrink-0 basis-full"
          >
            {slide.content}
          </div>
        ))}
      </div>

      {tone === 'dark' ? (
        count > 1 && (
          <div className="absolute inset-x-0 bottom-0 z-10 flex items-center justify-between gap-4 px-6 pb-6 md:px-14">
            {showDots ? (
              <div className="flex items-center gap-2">
                {slides.map((slide, i) => (
                  <button
                    key={slide.id}
                    type="button"
                    aria-label={slideLabel(i, count, slide.label)}
                    aria-current={i === index}
                    title={slide.label}
                    onClick={() => {
                      goTo(i, true)
                    }}
                    className={`h-1 rounded-full transition-all duration-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-white ${
                      i === index ? 'w-10 bg-white' : 'w-5 bg-white/35 hover:bg-white/60'
                    }`}
                  />
                ))}
              </div>
            ) : (
              <span />
            )}
            <div className="flex items-center gap-3">
              <span className="font-mono text-xs tracking-widest text-white/70" aria-hidden="true">
                {String(index + 1).padStart(2, '0')} / {String(count).padStart(2, '0')}
              </span>
              {showArrows && (
                <>
                  <button
                    type="button"
                    aria-label="Previous slide"
                    aria-controls={regionId}
                    disabled={atStart}
                    onClick={() => {
                      step('prev')
                    }}
                    className={glassArrow}
                  >
                    <Chevron direction="prev" />
                  </button>
                  <button
                    type="button"
                    aria-label="Next slide"
                    aria-controls={regionId}
                    disabled={atEnd}
                    onClick={() => {
                      step('next')
                    }}
                    className={glassArrow}
                  >
                    <Chevron direction="next" />
                  </button>
                </>
              )}
            </div>
          </div>
        )
      ) : (
        <>
          {showArrows && count > 1 && (
            <>
              <button
                type="button"
                aria-label="Previous slide"
                aria-controls={regionId}
                disabled={atStart}
                onClick={() => {
                  step('prev')
                }}
                className={`${arrowClass} left-3`}
              >
                <Chevron direction="prev" />
              </button>
              <button
                type="button"
                aria-label="Next slide"
                aria-controls={regionId}
                disabled={atEnd}
                onClick={() => {
                  step('next')
                }}
                className={`${arrowClass} right-3`}
              >
                <Chevron direction="next" />
              </button>
            </>
          )}
          {showDots && count > 1 && (
            <div className="absolute inset-x-0 bottom-3 z-10 flex justify-center gap-2">
              {slides.map((slide, i) => (
                <button
                  key={slide.id}
                  type="button"
                  aria-label={slideLabel(i, count, slide.label)}
                  aria-current={i === index}
                  title={slide.label}
                  onClick={() => {
                    goTo(i, true)
                  }}
                  className={`h-2.5 rounded-full transition-all duration-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                    i === index ? 'w-6 bg-gray-800' : 'w-2.5 bg-gray-300 hover:bg-gray-400'
                  }`}
                />
              ))}
            </div>
          )}
        </>
      )}

      <div className="sr-only" aria-live="polite">
        {announcement}
      </div>
    </section>
  )
}
