import { useSeen } from './useSeen'

// Shows its children with a short entrance when it scrolls into view (plan 262). Where the browser cannot tell (no IntersectionObserver, as in some
// test environments and old browsers) the children are shown at once, and the stylesheet shows everything at once under reduced motion. `delay` is
// the place in a row (each step is 80 ms later). The `is-in` class also starts the arrows grown inside (career-info-arrow).
export function Reveal({
  children,
  className = '',
  delay = 0,
  as: Tag = 'div',
}: {
  children: React.ReactNode
  className?: string
  delay?: number
  as?: 'div' | 'li' | 'section'
}) {
  const [ref, shown] = useSeen<HTMLElement>()
  const style = { '--d': delay } as React.CSSProperties
  return (
    <Tag
      ref={ref as never}
      style={style}
      className={`career-info-reveal ${shown ? 'is-in' : ''} ${className}`}
      data-shown={shown ? 'true' : 'false'}
    >
      {children}
    </Tag>
  )
}
