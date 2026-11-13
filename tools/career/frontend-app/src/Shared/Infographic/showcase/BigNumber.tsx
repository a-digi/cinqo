import { CountUp } from '../../Motion/CountUp'
import { prefersReducedMotion } from '../scrollRules'
import { useSeen } from '../useSeen'
import { toneColors, type ShowcaseTone } from './showcaseTokens'

// A very large number with a short line under it (the showcase look's "up to ..." claim). It counts up once, when it comes into view; under reduced motion, or where the browser cannot
// tell what is in view, it is simply there. A screen reader always reads the final text.
export function BigNumber({ value, label, tone }: { value: string; label: string; tone: ShowcaseTone }) {
  const [ref, seen] = useSeen<HTMLDivElement>()
  const c = toneColors(tone)
  const animate = !prefersReducedMotion()
  return (
    <div ref={ref} data-testid="big-number" className="text-center">
      <p aria-label={`${value} ${label}`} className="text-7xl font-semibold tracking-[-0.03em] md:text-9xl" style={{ color: c.color }}>
        <span aria-hidden="true">{seen || !animate ? <CountUp value={value} /> : '0'}</span>
      </p>
      <p className="mt-3 text-xl md:text-2xl" style={{ color: c.muted }}>
        {label}
      </p>
    </div>
  )
}
