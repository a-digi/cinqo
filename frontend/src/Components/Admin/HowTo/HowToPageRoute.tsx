import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { fetchPlatformKeys, fetchPlatforms, type Platform, type PlatformKey } from '../../../api/platforms'
import { ApiError } from '../../../api/client'
import { AppScopes } from '../../../config/security/scopes'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { ScopeGate } from '../../../Shared/Components/Access/ScopeGate'
import { formatBytes } from '../../../Shared/Components/Download/format'
import { fetchLocalAi, type LocalAiResponse } from '../LocalAi/localAiApi'
import { fitKind, fitLabel, needsSetup } from '../LocalAi/localAiFormat'
import { ModelPanel } from '../Platforms/PlatformSlides'
import { MOTIFS, TIER_BADGE } from '../Platforms/slideStyle'
import { infoFor, isSafeLink } from '../Platforms/platformInfo'
import { contentFor } from './HowToContent'
import { HowToLandingPage, type HowToHero } from './HowToLandingPage'
import { howToPath } from './howToRules'

const FIT_CLASS = { fits: 'text-emerald-700', tight: 'text-amber-700', unknown: 'text-gray-500' } as const

const lightButton = 'rounded-full bg-white px-5 py-2.5 text-sm font-semibold text-gray-900 shadow-lg transition hover:bg-white/90'
const ghostButton = 'rounded-full px-5 py-2.5 text-sm font-semibold text-white ring-1 ring-white/40 transition hover:bg-white/10'

function NotFound() {
  return (
    <div className="space-y-3 p-6">
      <Link to={howToPath('overview')} className="text-sm text-gray-500 hover:text-gray-900 hover:underline">
        ← How To: Overview
      </Link>
      <p className="text-sm text-gray-700">There is no How To page with this name.</p>
    </div>
  )
}

// /admin/ai/how-to/:pageId: the overview, the local agents, or one platform. It loads what the slides load (the platforms, the keys, the local
// agent's state) and hands the landing page its words and its live parts.
export function HowToPageRoute() {
  const { pageId = '' } = useParams()
  const navigate = useNavigate()
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [keys, setKeys] = useState<PlatformKey[]>([])
  const [local, setLocal] = useState<LocalAiResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    Promise.all([fetchPlatforms(), fetchPlatformKeys()])
      .then(([p, k]) => {
        if (cancelled) return
        setPlatforms(p)
        setKeys(k)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load platforms.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (pageId !== 'local-agents') return
    let cancelled = false
    fetchLocalAi()
      .then((d) => {
        if (!cancelled) setLocal(d)
      })
      .catch(() => undefined) // the page still reads; only the live state is missing
    return () => {
      cancelled = true
    }
  }, [pageId])

  if (error) return <p className="p-6 text-sm text-red-600">{error}</p>
  if (!platforms) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading…" />
      </div>
    )
  }

  const platform = platforms.find((p) => p.id === pageId && p.requiresKey)
  const content = contentFor(pageId, platform?.name)
  if (!content) return <NotFound />

  if (pageId === 'overview') {
    const hero: HowToHero = {
      from: '#4f46e5',
      to: '#0ea5e9',
      motif: 'sparkle',
      actions: (
        <ScopeGate scopes={[AppScopes.PlatformRead]}>
          <button
            type="button"
            onClick={() => {
              void navigate('/admin/api-keys')
            }}
            className={lightButton}
          >
            Open Remote Platforms
          </button>
          <button
            type="button"
            onClick={() => {
              void navigate('/admin/local-ai')
            }}
            className={ghostButton}
          >
            Open Local Agents
          </button>
        </ScopeGate>
      ),
    }
    return <HowToLandingPage content={content} hero={hero} />
  }

  if (pageId === 'local-agents') {
    const on = local?.settings.enabled === true && local.status.state === 'configured'
    const state = local
      ? on
        ? 'On'
        : local.status.state === 'preparing'
          ? 'Setting up…'
          : needsSetup(local)
            ? 'Needs a download'
            : 'Off'
      : ''
    const hero: HowToHero = {
      from: '#0d9488',
      to: '#0369a1',
      motif: 'network',
      badges: (
        <>
          <span className="rounded-full bg-emerald-400/20 px-2.5 py-0.5 text-xs font-medium text-emerald-100 ring-1 ring-emerald-300/40">
            No cost per use
          </span>
          {state && (
            <span className="rounded-full bg-white/10 px-2.5 py-0.5 text-xs font-medium text-white ring-1 ring-white/30">{state}</span>
          )}
        </>
      ),
      actions: (
        <button
          type="button"
          onClick={() => {
            void navigate('/admin/local-ai')
          }}
          className={lightButton}
        >
          Open Local Agents settings
        </button>
      ),
    }
    const models = local ? (
      <ul className="space-y-2">
        {local.catalog.map((m) => (
          <li key={m.id} className="flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
            <span className="font-mono text-xs text-gray-800">
              {m.name} · {formatBytes(m.sizeBytes)}
            </span>
            <span className={`text-xs ${FIT_CLASS[fitKind(m.fits)]}`}>{fitLabel(m.fits)}</span>
          </li>
        ))}
      </ul>
    ) : (
      <p className="text-sm text-gray-500">Loading…</p>
    )
    return <HowToLandingPage content={content} hero={hero} models={models} modelsTitle="Models Cinqo will manage" />
  }

  // a platform: the colours, badges, key link and models are the slide's own
  const info = infoFor(pageId, platform?.name ?? content.label)
  const keyCount = keys.filter((k) => k.platform === pageId).length
  const hero: HowToHero = {
    from: info.from,
    to: info.to,
    motif: MOTIFS[pageId] ?? 'rings',
    badges: info.tiers.map((tier) => (
      <span key={tier} className={`rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ${TIER_BADGE[tier].className}`}>
        {TIER_BADGE[tier].label}
      </span>
    )),
    actions: (
      <>
        {platform && (
          <ScopeGate scopes={[AppScopes.PlatformManage]}>
            <button
              type="button"
              onClick={() => {
                void navigate(`/admin/api-keys?platform=${encodeURIComponent(pageId)}`)
              }}
              className={lightButton}
            >
              Add a key for {platform.name}
            </button>
          </ScopeGate>
        )}
        {isSafeLink(info.keyUrl) && (
          <a href={info.keyUrl} target="_blank" rel="noopener noreferrer" className={ghostButton}>
            {info.keyLabel} ↗
          </a>
        )}
        {keyCount > 0 && (
          <span className="text-sm text-emerald-200">
            ✓ {keyCount} {keyCount === 1 ? 'key' : 'keys'} added
          </span>
        )}
      </>
    ),
  }
  const models = platform ? (
    <div className="rounded-2xl bg-slate-900 p-1 text-white">
      <ModelPanel models={platform.models} />
    </div>
  ) : (
    <p className="text-sm text-gray-500">This platform is not registered in this Cinqo, so there are no models to list.</p>
  )
  return <HowToLandingPage content={content} hero={hero} models={models} />
}
