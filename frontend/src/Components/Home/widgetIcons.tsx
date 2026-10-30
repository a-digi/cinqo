import type { WidgetIcon } from './widgetTheme'

// Stroke icons drawn in the current colour, 24x24.
const paths: Record<WidgetIcon, React.ReactNode> = {
  tools: <path d="M14.7 6.3a4 4 0 0 0-5.4 5.4L3.5 17.5l3 3 5.8-5.8a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.2-.6-.6-2.2z" />,
  updates: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 16V8m0 0-3.5 3.5M12 8l3.5 3.5" />
    </>
  ),
  chat: <path d="M20 12a8 8 0 0 1-11.6 7.1L4 20l1-4.1A8 8 0 1 1 20 12z" />,
  media: (
    <>
      <rect x="3.5" y="4.5" width="17" height="15" rx="3" />
      <circle cx="9" cy="10" r="1.6" />
      <path d="m4.5 17.5 4.5-4.5 3.5 3.5 3-3 4 4" />
    </>
  ),
  briefcase: (
    <>
      <rect x="3.5" y="7.5" width="17" height="12" rx="2.5" />
      <path d="M9 7.5V6a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v1.5M3.5 13h17" />
    </>
  ),
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c2.6 2.7 3.8 5.7 3.8 9s-1.2 6.3-3.8 9c-2.6-2.7-3.8-5.7-3.8-9S9.4 5.7 12 3z" />
    </>
  ),
  chart: <path d="M5 20V11m5 9V5m5 15v-7m5 7V9" />,
  list: <path d="M9 6.5h11M9 12h11M9 17.5h11M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01" />,
  number: <path d="M9.5 4 8 20M16 4l-1.5 16M4.5 9h16M3.5 15h16" />,
  puzzle: <path d="M10 4a2 2 0 1 1 4 0v2h4v4h-2a2 2 0 1 0 0 4h2v4h-4v-2a2 2 0 1 0-4 0v2H6v-4h2a2 2 0 1 0 0-4H6V6h4z" />,
}

export function WidgetIconGlyph({ name, className = 'h-5 w-5' }: { name: WidgetIcon; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
    >
      {paths[name]}
    </svg>
  )
}

// The gradient squircle an icon sits in, shared by the cards and the Customize list.
export function IconBadge({ name, from, to, size = 'md' }: { name: WidgetIcon; from: string; to: string; size?: 'sm' | 'md' }) {
  const box = size === 'sm' ? 'h-8 w-8 rounded-xl' : 'h-10 w-10 rounded-2xl'
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center text-white shadow-sm ${box}`}
      style={{ background: `linear-gradient(135deg, ${from}, ${to})` }}
    >
      <WidgetIconGlyph name={name} className={size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'} />
    </span>
  )
}
