import type { Accent } from '../../Components/JobsPage/jobsTheme'
import { useSeen } from './useSeen'

// An arrow made of HTML and CSS (a line and a head): it grows once when it comes into view. `down` points down (used where a row wraps).
export function Arrow({ accent, down = false, className = '' }: { accent: Accent; down?: boolean; className?: string }) {
  // It starts growing when it itself comes into view (it may sit outside any revealed block).
  const [ref, seen] = useSeen<HTMLSpanElement>()
  return (
    <span
      ref={ref}
      aria-hidden="true"
      className={`career-info-arrow shrink-0 ${seen ? 'is-in' : ''} ${down ? 'is-down' : ''} ${className}`}
      style={{ color: accent.to }}
    >
      <i className="career-info-arrow-line" />
      <i className="career-info-arrow-head" />
    </span>
  )
}
