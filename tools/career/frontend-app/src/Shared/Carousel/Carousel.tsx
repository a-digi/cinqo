import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from 'react'
import {
  autoPlayInterval,
  showsPauseButton,
  clampIndex,
  keyDirection,
  slideLabel,
  slideOffset,
  stepIndex,
  swipeDirection,
  type Direction,
  type Orientation,
} from './carouselRules'
import { PAGE_ACCENT, type Accent } from '../../Components/JobsPage/jobsTheme'
import { Decor } from '../Decor/Decor'
import { shapeAt, type DecorShape } from '../Decor/decorShapes'

export interface CarouselSlide {
  // Stable key for the slide.
  id: string
  // A short name: the dot's tooltip and what a screen reader hears ("2 of 4: Paywalls").
  label: string
  content: ReactNode
  // The slide's colour: its card tint, its shapes, and the arrows and dots while it is shown. The page's blue when omitted.
  accent?: Accent
  // The shape behind the slide; when omitted the slides take turns (circles, diamond, ring).
  shape?: DecorShape
}

export interface CarouselProps {
  slides: CarouselSlide[]
  // The frame's accessible name ("Tips for crawling portals").
  ariaLabel: string
  // 'horizontal' (default): slides move sideways, arrows at the edges, dots underneath, Left/Right keys, swipe.
  // 'vertical': slides move up and down, a control column on the right (up arrow, dots, down arrow), Up/Down
  // keys, no swipe (a vertical drag is the page scrolling).
  orientation?: Orientation
  // Controlled use: the page owns the position. Leave both out and the carousel keeps its own.
  index?: number
  onIndexChange?: (index: number) => void
  // Wrap from the last slide to the first (and back).
  loop?: boolean
  // Milliseconds between automatic slides; 0 (default) means it never moves on its own. AUTO_PLAY_MS has the usual
  // times (3000, 5000, 8000); any value from 2000 up works. It pauses while the pointer or keyboard focus is inside,
  // can be paused with a button of its own (shown only while it is on), waits a full period again after every move,
  // and never runs under prefers-reduced-motion. Use loop with it, so it goes round instead of stopping at the last slide.
  autoPlayMs?: number
  showArrows?: boolean
  showDots?: boolean
  className?: string
}

const SLIDE_MS = 600
const EASE = 'cubic-bezier(0.22, 0.61, 0.36, 1)'

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

function Chevron({ direction, orientation }: { direction: Direction; orientation: Orientation }) {
  const path =
    orientation === 'horizontal'
      ? direction === 'next'
        ? 'M7.5 4.5 13 10l-5.5 5.5'
        : 'M12.5 4.5 7 10l5.5 5.5'
      : direction === 'next'
        ? 'M4.5 7.5 10 13l5.5-5.5'
        : 'M4.5 12.5 10 7l5.5 5.5'
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4" aria-hidden="true">
      <path d={path} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

const ARROW_LABEL: Record<Orientation, Record<Direction, string>> = {
  horizontal: { prev: 'Previous slide', next: 'Next slide' },
  vertical: { prev: 'Previous slide (up)', next: 'Next slide (down)' },
}

// A shared carousel: one slide at a time, arrows, dots, the arrow keys, and (horizontal only) swipe. Purely
// presentational: it takes slides and shows them, no data, no fetching. Every slide sits in a card like the
// dashboard cards (a pale tint of its accent, a soft border, shapes behind it), and the arrows and dots take the
// colour of the slide that is shown.
//
// The slides are stacked in one grid cell and each is moved by its distance from the shown one, so the frame is
// as tall as the tallest slide with no height to set, and nothing jumps when the slide changes. The ones not shown
// are aria-hidden and inert, so focus and screen readers skip them. The slides move slowly (600 ms) and not at
// all under prefers-reduced-motion.
//
//   <Carousel orientation="vertical" ariaLabel="Tips" slides={[{ id: 'a', label: 'First', content: <p>…</p> }]} />
//
// The horizontal design puts its arrows over the left and right edges of the card, so the card keeps 3rem of side
// padding for them; the vertical one keeps its controls in their own column and needs none.
//
// Use index + onIndexChange to control it from the page. See
// plan/ai/tools/career/step-105-shared-carousel-and-portal-tips.md.
export function Carousel({
  slides,
  ariaLabel,
  orientation = 'horizontal',
  index: controlledIndex,
  onIndexChange,
  loop = false,
  autoPlayMs = 0,
  showArrows = true,
  showDots = true,
  className = '',
}: CarouselProps) {
  const count = slides.length
  const vertical = orientation === 'vertical'
  const [ownIndex, setOwnIndex] = useState(0)
  const index = clampIndex(controlledIndex ?? ownIndex, count)
  const [paused, setPaused] = useState(false)
  // Paused with the button (the pointer and focus pause it too, through `paused`).
  const [userPaused, setUserPaused] = useState(false)
  // Said to screen readers after the user (not the timer) moved the carousel.
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

  // Swipe is for the horizontal design only; where a drag starts is remembered until it ends.
  const dragStart = useRef<{ x: number; y: number } | null>(null)
  const swipe = vertical
    ? {}
    : {
        onPointerDown: (e: PointerEvent) => {
          dragStart.current = { x: e.clientX, y: e.clientY }
        },
        onPointerUp: (e: PointerEvent) => {
          const start = dragStart.current
          dragStart.current = null
          if (!start) return
          const direction = swipeDirection(e.clientX - start.x, e.clientY - start.y)
          if (direction) step(direction)
        },
        onPointerCancel: () => {
          dragStart.current = null
        },
      }

  const interval = autoPlayInterval(autoPlayMs, count, reducedMotion, paused || userPaused)
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
  const canAutoPlay = showsPauseButton(autoPlayMs, count, reducedMotion)
  const atStart = !loop && index === 0
  const atEnd = !loop && index === count - 1
  const activeAccent = slides[index]?.accent ?? PAGE_ACCENT
  // The arrows and the active dot wear the colour of the slide that is shown.
  const arrowStyle = { color: activeAccent.text, '--tw-ring-color': activeAccent.from } as CSSProperties

  // The arrows and the pause button are hidden until the pointer is over the carousel (the dots always show). They also
  // show while keyboard focus is anywhere inside it, so a keyboard user can see what they have tabbed to, and always on a
  // screen that cannot hover (touch), where they would otherwise never appear. A disabled one shows faintly, as before.
  const revealOnHover =
    'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 group-hover:disabled:opacity-30 group-focus-within:disabled:opacity-30 [@media(hover:none)]:opacity-100 [@media(hover:none)]:disabled:opacity-30'
  const arrowBase = `flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-white/95 shadow-sm ring-1 transition hover:bg-white focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:pointer-events-none ${revealOnHover}`
  const edgeArrow = (direction: Direction, disabled: boolean) => (
    <button
      type="button"
      aria-label={ARROW_LABEL.horizontal[direction]}
      aria-controls={regionId}
      disabled={disabled}
      onClick={() => {
        step(direction)
      }}
      style={arrowStyle}
      className={`${arrowBase} absolute top-1/2 z-10 -translate-y-1/2 ${direction === 'prev' ? 'left-2' : 'right-2'}`}
    >
      <Chevron direction={direction} orientation="horizontal" />
    </button>
  )
  const columnArrow = (direction: Direction, disabled: boolean) => (
    <button
      type="button"
      aria-label={ARROW_LABEL.vertical[direction]}
      aria-controls={regionId}
      disabled={disabled}
      onClick={() => {
        step(direction)
      }}
      style={arrowStyle}
      className={`${arrowBase} h-7 w-7`}
    >
      <Chevron direction={direction} orientation="vertical" />
    </button>
  )

  const dots = (
    <div className={vertical ? 'flex flex-col items-center gap-1.5' : 'flex items-center justify-center gap-1.5'}>
      {slides.map((slide, i) => (
        <button
          key={slide.id}
          type="button"
          aria-label={slideLabel(i, count, slide.label)}
          aria-current={i === index}
          aria-controls={regionId}
          title={slide.label}
          onClick={() => {
            goTo(i, true)
          }}
          style={{ background: i === index ? activeAccent.to : activeAccent.from, opacity: i === index ? 1 : 0.35 }}
          className={`rounded-full transition-all duration-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
            vertical ? (i === index ? 'h-6 w-1.5' : 'h-3 w-1.5') : i === index ? 'h-1.5 w-6' : 'h-1.5 w-3'
          }`}
        />
      ))}
    </div>
  )

  // Stops and restarts the automatic sliding for good (until pressed again), so it can always be stopped, not only
  // by holding the pointer over it. Shown only while the carousel moves by itself.
  const pauseButton = canAutoPlay ? (
    <button
      type="button"
      aria-label={userPaused ? 'Resume automatic sliding' : 'Pause automatic sliding'}
      aria-pressed={userPaused}
      title={userPaused ? 'Resume automatic sliding' : 'Pause automatic sliding'}
      onClick={() => {
        setUserPaused((v) => !v)
      }}
      style={arrowStyle}
      className={`${arrowBase} ${vertical ? 'h-7 w-7' : 'h-6 w-6'}`}
    >
      <svg viewBox="0 0 20 20" fill="currentColor" className="h-3 w-3" aria-hidden="true">
        {userPaused ? <path d="M6 4.5v11l9-5.5z" /> : <path d="M5.5 4h3.2v12H5.5zM11.3 4h3.2v12h-3.2z" />}
      </svg>
    </button>
  ) : null

  const slideStyle = (i: number): CSSProperties => {
    const offset = slideOffset(i, index) * 100
    return {
      gridArea: '1 / 1',
      transform: vertical ? `translateY(${offset}%)` : `translateX(${offset}%)`,
      transition: reducedMotion ? 'none' : `transform ${SLIDE_MS}ms ${EASE}`,
    }
  }

  const frame = (
    <div id={regionId} className="grid overflow-hidden" style={vertical ? undefined : { touchAction: 'pan-y' }} {...swipe}>
      {slides.map((slide, i) => (
        <div
          key={slide.id}
          role="group"
          aria-roledescription="slide"
          aria-label={slideLabel(i, count, slide.label)}
          aria-hidden={i !== index}
          inert={i !== index}
          data-slide-active={i === index}
          className="min-w-0"
          style={slideStyle(i)}
        >
          <div
            className={`relative h-full overflow-hidden rounded-3xl border shadow-sm ${vertical ? 'p-4' : 'px-12 py-5'}`}
            style={{
              borderColor: (slide.accent ?? PAGE_ACCENT).soft,
              background: `linear-gradient(160deg, #ffffff 40%, ${(slide.accent ?? PAGE_ACCENT).soft} 100%)`,
            }}
          >
            <Decor shape={slide.shape ?? shapeAt(i)} accent={slide.accent ?? PAGE_ACCENT} />
            <div className="relative">{slide.content}</div>
          </div>
        </div>
      ))}
    </div>
  )

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const direction = keyDirection(e.key, orientation)
    if (!direction) return
    e.preventDefault()
    step(direction)
  }

  return (
    // The frame is focusable and handles the arrow keys on purpose: that is the keyboard behaviour of the WAI-ARIA
    // carousel pattern (a region that moves with the arrow keys of its axis), which this rule cannot express.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <section
      aria-roledescription="carousel"
      aria-label={ariaLabel}
      className={`group relative outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${className}`}
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
      onKeyDown={onKeyDown}
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
      {vertical ? (
        <div className="flex items-stretch gap-2">
          <div className="min-w-0 flex-1">{frame}</div>
          {count > 1 && (showArrows || showDots || canAutoPlay) && (
            <div className="flex shrink-0 flex-col items-center justify-center gap-2">
              {showArrows && columnArrow('prev', atStart)}
              {showDots && dots}
              {showArrows && columnArrow('next', atEnd)}
              {pauseButton}
            </div>
          )}
        </div>
      ) : (
        <>
          <div className="relative">
            {frame}
            {count > 1 && showArrows && (
              <>
                {edgeArrow('prev', atStart)}
                {edgeArrow('next', atEnd)}
              </>
            )}
          </div>
          {count > 1 && (showDots || canAutoPlay) && (
            <div className="mt-3 flex items-center justify-center gap-3">
              {showDots && dots}
              {pauseButton}
            </div>
          )}
        </>
      )}
      <p className="sr-only" aria-live="polite">
        {announcement}
      </p>
    </section>
  )
}
