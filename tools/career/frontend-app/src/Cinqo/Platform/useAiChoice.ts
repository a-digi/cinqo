import { useCallback, useEffect, useState } from 'react'
import { fetchAiSettings, type AiSettings } from '../../api'
import { resolveAiChoice, type AiChoice } from './aiChoice'
import { fetchPlatformKeys, fetchPlatforms, type Platform } from './platformRepository'

export interface UseAiChoice extends AiChoice {
  platforms: Platform[]
  // What is saved (empty strings when nothing is), for the Settings page.
  saved: AiSettings
  // False until the first load has finished (so a page can wait before it acts).
  loaded: boolean
  reload: () => void
}

const NOTHING_SAVED: AiSettings = { platformId: '', model: '' }

// The tool-wide AI platform and model, loaded from core (the platforms and which have
// keys) and from this tool (the saved choice). Every failure degrades to "not chosen
// here": a key list or settings load failure falls back to the automatic pick, and no
// platforms at all gives platform null, which every caller already handles (a button
// disabled, or a clear error at the point it matters).
export function useAiChoice(): UseAiChoice {
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [keyed, setKeyed] = useState<string[]>([])
  const [saved, setSaved] = useState<AiSettings>(NOTHING_SAVED)
  const [loaded, setLoaded] = useState(false)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    void Promise.all([
      fetchPlatforms().catch(() => [] as Platform[]),
      fetchPlatformKeys().catch(() => []),
      fetchAiSettings().catch(() => NOTHING_SAVED),
    ])
      .then(([list, keys, settings]) => {
        if (cancelled) return
        setPlatforms(list)
        setKeyed(keys.map((k) => k.platform))
        setSaved(settings)
      })
      .finally(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [attempt])

  const reload = useCallback(() => {
    setAttempt((n) => n + 1)
  }, [])

  return { ...resolveAiChoice(platforms, keyed, saved), platforms, saved, loaded, reload }
}
