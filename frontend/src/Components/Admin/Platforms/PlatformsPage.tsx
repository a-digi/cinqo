import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { fetchPlatforms, fetchPlatformKeys, type Platform, type PlatformKey } from '../../../api/platforms'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { PlatformSlides } from './PlatformSlides'

function countByPlatform(keys: PlatformKey[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const k of keys) counts[k.platform] = (counts[k.platform] ?? 0) + 1
  return counts
}

// The AI platforms, explained in a slider. Keys are managed on their own page (/admin/api-keys); a slide's
// "Add a key for …" button goes there with the platform preselected.
export function PlatformsPage() {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [keys, setKeys] = useState<PlatformKey[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    Promise.all([fetchPlatforms(), fetchPlatformKeys()])
      .then(([p, k]) => {
        setPlatforms(p)
        setKeys(k)
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Failed to load platforms.')
      })
  }, [])

  if (error) return <p className="p-6 text-sm text-red-600">{error}</p>

  if (!platforms || !keys) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading platforms…" />
      </div>
    )
  }

  return (
    <div className="cinqo-stagger cinqo-v-zoom cinqo-t-shimmer space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Platforms</h1>
        <p className="mt-1 text-sm text-gray-500">The AI platforms Cinqo can use, and which models they offer.</p>
      </div>
      <PlatformSlides
        platforms={platforms}
        keyCounts={countByPlatform(keys)}
        onAddKey={(id) => {
          void navigate(`/admin/api-keys?platform=${encodeURIComponent(id)}`)
        }}
      />
    </div>
  )
}
