import { LANDING } from './landingTokens'

// A picture in a thin royal-blue outlined frame (the landing look's product frame), with an optional round badge on its corner ("%", "+", a step number).
export function BlueFrame({ children, badge, className = '' }: { children: React.ReactNode; badge?: string; className?: string }) {
  return (
    <div
      className={`relative rounded-2xl border-2 bg-white p-3 ${className}`}
      style={{ borderColor: LANDING.outline, boxShadow: `0 0 0 4px ${LANDING.outlineSoft}` }}
      data-testid="blue-frame"
    >
      {children}
      {badge !== undefined && (
        <span
          data-testid="frame-badge"
          aria-hidden="true"
          className="absolute -right-3 -top-3 inline-flex h-8 w-8 items-center justify-center rounded-full border-2 bg-white text-sm font-extrabold"
          style={{ borderColor: LANDING.outline, color: LANDING.outline }}
        >
          {badge}
        </span>
      )}
    </div>
  )
}
