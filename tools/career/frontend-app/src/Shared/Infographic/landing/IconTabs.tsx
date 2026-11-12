import { LANDING } from './landingTokens'

export interface IconTab {
  id: string
  label: string
  icon: React.ReactNode
  // A small figure on the tab ("3/4"), or null.
  badge?: string | null
  // The chapter's colour: the icon and the underline take it while the tab is current.
  color?: string
}

// The tab strip under the hero: a line icon over a short label for each chapter. The chapter in view has a pale-grey top and a thick black underline that
// slides in. It sticks to the top of the scrolling poster and scrolls sideways on a narrow window.
export function IconTabs({ tabs, current, onGo }: { tabs: IconTab[]; current: number; onGo: (id: string) => void }) {
  return (
    <nav
      aria-label="Chapters"
      className="sticky -top-5 z-10 -mx-6 mb-8 border-b border-gray-200 bg-white/95 px-6 backdrop-blur"
      data-testid="icon-tabs"
    >
      <ul className="mx-auto flex max-w-6xl justify-start gap-1 overflow-x-auto md:justify-center">
        {tabs.map((t, i) => {
          const active = i === current
          return (
            <li key={t.id} className="shrink-0">
              <button
                type="button"
                onClick={() => {
                  onGo(t.id)
                }}
                aria-current={active ? 'true' : undefined}
                className="relative flex w-24 flex-col items-center gap-1.5 px-2 pb-3 pt-3 text-xs font-semibold transition-colors hover:bg-gray-50 motion-reduce:transition-none md:w-28"
                style={{ background: active ? LANDING.tile : undefined, color: LANDING.ink }}
              >
                <span
                  className={`h-7 w-7 transition-colors motion-reduce:transition-none ${active ? 'landing-bob' : ''}`}
                  style={{ color: active && t.color ? t.color : undefined }}
                  aria-hidden="true"
                >
                  {t.icon}
                </span>
                <span>
                  {t.label}
                  {t.badge !== undefined && t.badge !== null && (
                    <span className="ml-1 rounded-full bg-black/10 px-1.5 text-[10px]">{t.badge}</span>
                  )}
                </span>
                <span
                  data-testid={`tab-underline-${t.id}`}
                  aria-hidden="true"
                  className="absolute inset-x-0 bottom-0 h-[3px] origin-left transition-transform duration-300 motion-reduce:transition-none"
                  style={{ background: t.color ?? LANDING.ink, transform: active ? 'scaleX(1)' : 'scaleX(0)' }}
                />
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
