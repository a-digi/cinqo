import type { CSSProperties, ReactNode } from 'react'
import { Slider, type SliderSlide } from '../../../Shared/Components/Slider/Slider'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { AppScopes } from '../../../config/security/scopes'
import type { Platform } from '../../../api/platforms'
import { groupModels, infoFor, orderPlatforms, isSafeLink, modelDisplay, type Tier } from './platformInfo'

type Motif = 'sparkle' | 'rings' | 'burst' | 'network'

const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round' } as const

// Line art on the right of a slide: each platform gets its own shape, drawn in SVG so the slide needs no
// image files and stays sharp at any size.
function MotifArt({ motif }: { motif: Motif }) {
  const common = { fill: 'none', stroke: 'white', strokeWidth: 1 } as const
  return (
    <svg viewBox="0 0 400 400" className="h-full w-full" aria-hidden="true">
      {motif === 'rings' &&
        [60, 100, 140, 180].map((r, i) => <circle key={r} cx="200" cy="200" r={r} {...common} strokeOpacity={0.5 - i * 0.09} />)}
      {motif === 'burst' &&
        Array.from({ length: 36 }, (_, i) => {
          const a = (i / 36) * Math.PI * 2
          return (
            <line
              key={i}
              x1={200 + Math.cos(a) * 50}
              y1={200 + Math.sin(a) * 50}
              x2={200 + Math.cos(a) * (i % 2 ? 130 : 185)}
              y2={200 + Math.sin(a) * (i % 2 ? 130 : 185)}
              {...common}
              strokeOpacity={0.45}
            />
          )
        })}
      {motif === 'network' && (
        <g {...common} strokeOpacity={0.5}>
          <path d="M60 300 L150 180 L260 230 L340 100 M150 180 L170 70 L260 230 L330 320 M60 300 L170 70" />
          {[
            [60, 300],
            [150, 180],
            [260, 230],
            [340, 100],
            [170, 70],
            [330, 320],
          ].map(([x, y]) => (
            <circle key={`${x}-${y}`} cx={x} cy={y} r="7" fill="white" fillOpacity="0.8" stroke="none" />
          ))}
        </g>
      )}
      {motif === 'sparkle' && (
        <g {...common} strokeOpacity={0.55}>
          <path d="M200 40 L226 160 L350 200 L226 240 L200 360 L174 240 L50 200 L174 160 Z" />
          <path d="M200 90 L214 175 L300 200 L214 225 L200 310 L186 225 L100 200 L186 175 Z" strokeOpacity={0.4} />
        </g>
      )}
    </svg>
  )
}

// The slide's full-bleed backdrop: a dark base, two colour glows that drift slowly, a faint dot grid and the
// line art.
function Backdrop({ from, to, motif }: { from: string; to: string; motif: Motif }) {
  return (
    <div className="absolute inset-0 overflow-hidden" style={{ background: '#080b16' }} aria-hidden="true">
      <div
        className="cinqo-slide-drift absolute -left-1/4 -top-1/3 h-[130%] w-[75%] rounded-full opacity-70 blur-3xl"
        style={{ background: `radial-gradient(closest-side, ${from}, transparent)` }}
      />
      <div
        className="cinqo-slide-drift absolute -bottom-1/2 -right-1/4 h-[150%] w-[75%] rounded-full opacity-60 blur-3xl"
        style={{ background: `radial-gradient(closest-side, ${to}, transparent)`, animationDelay: '-7s' }}
      />
      <div
        className="absolute inset-0 opacity-30"
        style={{
          backgroundImage: 'radial-gradient(rgba(255,255,255,0.55) 1px, transparent 1px)',
          backgroundSize: '26px 26px',
          maskImage: 'linear-gradient(to right, transparent, black 70%)',
          WebkitMaskImage: 'linear-gradient(to right, transparent, black 70%)',
        }}
      />
      <div className="absolute -right-10 top-1/2 hidden h-[420px] w-[420px] -translate-y-1/2 text-white md:block">
        <MotifArt motif={motif} />
      </div>
      <div className="absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-transparent" />
    </div>
  )
}

function Frame({ from, to, motif, children }: { from: string; to: string; motif: Motif; children: ReactNode }) {
  return (
    <div className="relative h-full min-h-[460px] text-white">
      <Backdrop from={from} to={to} motif={motif} />
      <div className="relative px-6 pb-24 pt-10 md:px-14 md:pt-14">{children}</div>
    </div>
  )
}

// Entrance order for the text of a slide (see .cinqo-slide-in in index.css).
function In({ i, className = '', children }: { i: number; className?: string; children: ReactNode }) {
  return (
    <div className={`cinqo-slide-in ${className}`} style={{ '--i': i } as CSSProperties}>
      {children}
    </div>
  )
}

const TIER_BADGE: Record<Tier, { label: string; className: string }> = {
  free: { label: 'Free models', className: 'bg-emerald-400/20 text-emerald-100 ring-emerald-300/40' },
  premium: { label: 'Premium models', className: 'bg-violet-400/20 text-violet-100 ring-violet-300/40' },
}

function Eyebrow({ children }: { children: ReactNode }) {
  return <p className="text-xs font-semibold uppercase tracking-[0.2em] text-white/60">{children}</p>
}

function Title({ children }: { children: ReactNode }) {
  return <h2 className="mt-3 text-4xl font-semibold leading-[1.05] tracking-tight md:text-5xl">{children}</h2>
}

function Subtitle({ children }: { children: ReactNode }) {
  return <p className="mt-4 max-w-xl text-lg leading-relaxed text-white/75">{children}</p>
}

function IntroTile({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <div className="rounded-2xl bg-white/10 p-4 ring-1 ring-white/15 backdrop-blur-md">
      <span className="inline-flex h-9 w-9 items-center justify-center rounded-xl bg-white/15">{icon}</span>
      <p className="mt-3 text-sm font-semibold">{title}</p>
      <p className="mt-1 text-sm leading-relaxed text-white/70">{children}</p>
    </div>
  )
}

function IntroSlide({ count }: { count: number }) {
  return (
    <Frame from="#4f46e5" to="#0ea5e9" motif="sparkle">
      <In i={0}>
        <Eyebrow>
          {count} {count === 1 ? 'platform' : 'platforms'} · swipe to explore
        </Eyebrow>
      </In>
      <In i={1}>
        <Title>
          Bring your own AI,
          <br />
          free or premium.
        </Title>
      </In>
      <In i={2}>
        <Subtitle>
          A platform is the company whose models answer your conversations. Connect one with your own API key. Some offer free models, many
          are premium.
        </Subtitle>
      </In>
      <In i={3} className="mt-8 grid max-w-3xl gap-3 md:grid-cols-3">
        <IntroTile
          title="Pick a platform"
          icon={
            <svg viewBox="0 0 24 24" className="h-5 w-5" {...stroke} aria-hidden="true">
              <rect x="4" y="5" width="16" height="6" rx="1.5" />
              <rect x="4" y="13" width="16" height="6" rx="1.5" />
            </svg>
          }
        >
          OpenAI, Anthropic and OpenRouter are ready.
        </IntroTile>
        <IntroTile
          title="Free or premium"
          icon={
            <svg viewBox="0 0 24 24" className="h-5 w-5" {...stroke} aria-hidden="true">
              <path d="M12 3v18M16.5 7.5c-.7-1.2-2.2-2-4.5-2-2.7 0-4.2 1.2-4.2 3 0 4.5 8.7 2.2 8.7 6.7 0 1.8-1.7 3.3-4.5 3.3-2.4 0-4-.9-4.7-2.3" />
            </svg>
          }
        >
          OpenRouter offers both; premium is billed per use.
        </IntroTile>
        <IntroTile
          title="Your key stays safe"
          icon={
            <svg viewBox="0 0 24 24" className="h-5 w-5" {...stroke} aria-hidden="true">
              <rect x="5" y="10.5" width="14" height="9.5" rx="2" />
              <path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" />
            </svg>
          }
        >
          Encrypted on your server, only ever shown masked.
        </IntroTile>
      </In>
    </Frame>
  )
}

const MOTIFS: Record<string, Motif> = { openai: 'rings', anthropic: 'burst', openrouter: 'network' }

function ModelPanel({ models }: { models: string[] }) {
  const groups = groupModels(models)
  return (
    <div className="rounded-2xl bg-white/10 p-5 ring-1 ring-white/15 backdrop-blur-md">
      {groups.free.length > 0 && (
        <>
          <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-200">Free</p>
          <div className="mb-4 mt-2 flex flex-wrap gap-2">
            {groups.free.map((m) => (
              <span
                key={m}
                className="rounded-full bg-emerald-400/20 px-3 py-1 font-mono text-xs text-emerald-50 ring-1 ring-emerald-300/40"
              >
                {modelDisplay(m)}
              </span>
            ))}
          </div>
        </>
      )}
      <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-violet-200">Premium</p>
      {groups.premium.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-2">
          {groups.premium.map((m) => (
            <span key={m} className="rounded-full bg-white/10 px-3 py-1 font-mono text-xs text-white/90 ring-1 ring-white/20">
              {m}
            </span>
          ))}
        </div>
      ) : (
        <p className="mt-2 text-sm text-white/60">No premium models are offered here.</p>
      )}
    </div>
  )
}

function PlatformSlide({
  platform,
  position,
  keyCount,
  onAddKey,
}: {
  platform: Platform
  position: number
  keyCount: number
  onAddKey: (platformId: string) => void
}) {
  const info = infoFor(platform.id, platform.name)
  const showLink = isSafeLink(info.keyUrl)
  return (
    <Frame from={info.from} to={info.to} motif={MOTIFS[platform.id] ?? 'rings'}>
      <div className="grid items-center gap-8 md:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <div>
          <In i={0} className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-semibold uppercase tracking-[0.2em] text-white/60">
              Platform {String(position).padStart(2, '0')}
            </span>
            {info.tiers.map((tier) => (
              <span key={tier} className={`rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ${TIER_BADGE[tier].className}`}>
                {TIER_BADGE[tier].label}
              </span>
            ))}
          </In>
          <In i={1}>
            <Title>{platform.name}</Title>
          </In>
          <In i={2}>
            <Subtitle>{info.tagLine}</Subtitle>
          </In>
          <In i={3}>
            <p className="mt-3 max-w-xl text-sm leading-relaxed text-white/60">
              <span className="font-semibold text-white/80">Good for: </span>
              {info.goodFor} {info.note}
            </p>
          </In>
          <In i={4} className="mt-6 flex flex-wrap items-center gap-3">
            <ScopeGate scopes={[AppScopes.PlatformManage]}>
              <button
                type="button"
                onClick={() => {
                  onAddKey(platform.id)
                }}
                className="rounded-full bg-white px-5 py-2.5 text-sm font-semibold text-gray-900 shadow-lg transition hover:bg-white/90"
              >
                Add a key for {platform.name}
              </button>
            </ScopeGate>
            {showLink && (
              <a
                href={info.keyUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-full px-5 py-2.5 text-sm font-semibold text-white ring-1 ring-white/40 transition hover:bg-white/10"
              >
                {info.keyLabel} ↗
              </a>
            )}
            {keyCount > 0 && (
              <span className="text-sm text-emerald-200">
                ✓ {keyCount} {keyCount === 1 ? 'key' : 'keys'} added
              </span>
            )}
          </In>
        </div>
        <In i={3}>
          <ModelPanel models={platform.models} />
        </In>
      </div>
    </Frame>
  )
}

// The slider above the keys table on the Platforms page: an overview slide, then one slide per
// registered platform (a platform the content file does not know still gets a generic one).
export function PlatformSlides({
  platforms,
  keyCounts,
  onAddKey,
}: {
  platforms: Platform[]
  keyCounts: Record<string, number>
  onAddKey: (platformId: string) => void
}) {
  const slides: SliderSlide[] = [
    { id: 'overview', label: 'Overview', content: <IntroSlide count={platforms.length} /> },
    ...orderPlatforms(platforms).map((p, i) => ({
      id: p.id,
      label: p.name,
      content: <PlatformSlide platform={p} position={i + 1} keyCount={keyCounts[p.id] ?? 0} onAddKey={onAddKey} />,
    })),
  ]
  return <Slider ariaLabel="About AI platforms" slides={slides} tone="dark" className="rounded-3xl shadow-xl" />
}
