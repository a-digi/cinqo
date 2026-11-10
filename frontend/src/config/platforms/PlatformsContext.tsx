import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ApiError } from '../../api/client'
import { fetchPlatforms, type Platform } from '../../api/platforms'
import { PLATFORMS_LOAD_ERROR, followUpDelay, samePlatforms, shouldReadOnReturn } from './platformsRules'
import { createRefresher } from './platformsRefresher'

// Why the AI platforms may have changed: an API key was added or removed, or the local AI (its models, its switch) changed.
export type PlatformsChange = 'keys' | 'local-ai'

export interface PlatformsContextValue {
  // The platforms the shell knows (GET /api/v1/platforms), or null until the first read has worked. A later read that fails keeps the list.
  platforms: Platform[] | null
  // Why the first read failed; null once there is a list (a failed re-read never replaces a list with an error).
  error: string | null
  // Goes up every time a read brings a different list; an identical read changes nothing.
  version: number
  // Reads the platforms again (single-flight).
  refresh: () => Promise<void>
  // What a page that changed keys or the local AI calls: the platforms are read again at once.
  notifyChanged: (reason: PlatformsChange) => void
}

const PlatformsContext = createContext<PlatformsContextValue | null>(null)

// Context + hook co-located deliberately (the convention of every *Context.tsx here): costs this file Fast Refresh for the hook, never a runtime issue.
// eslint-disable-next-line react-refresh/only-export-components
export function usePlatforms(): PlatformsContextValue {
  const value = useContext(PlatformsContext)
  if (value === null) throw new Error('usePlatforms must be used within a PlatformsProvider')
  return value
}

// The one list of AI platforms for the whole shell (plan/ai/frontend/step-243-ai-availability-context-chats-follow-keys-and-local-models.md): the floating
// chat and the conversations page read it instead of fetching their own copy once, so they see a key or a model that was added or removed.
export function PlatformsProvider({ children }: { children: ReactNode }) {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [version, setVersion] = useState(0)
  const current = useRef<Platform[] | null>(null)
  const alive = useRef(true)

  const refresh = useMemo(
    () =>
      createRefresher<Platform[]>({
        read: fetchPlatforms,
        onResult: (list) => {
          if (!alive.current) return
          setError(null)
          if (samePlatforms(current.current, list)) return
          current.current = list
          setPlatforms(list)
          setVersion((v) => v + 1)
        },
        onError: (err) => {
          // a failed read keeps the list it had; only a missing first list is an error
          if (!alive.current || current.current !== null) return
          setError(err instanceof ApiError ? err.message : PLATFORMS_LOAD_ERROR)
        },
      }),
    [],
  )

  useEffect(() => {
    alive.current = true
    void refresh()
    return () => {
      alive.current = false
    }
  }, [refresh])

  // Coming back to the window or the tab (a key added in another tab, a download that finished meanwhile): the platforms are read again, once for the
  // burst of events that coming back fires.
  const lastReturn = useRef<number | null>(null)
  useEffect(() => {
    function onReturn() {
      if (document.visibilityState !== 'visible') return
      const now = Date.now()
      if (!shouldReadOnReturn(now, lastReturn.current)) return
      lastReturn.current = now
      void refresh()
    }
    window.addEventListener('focus', onReturn)
    document.addEventListener('visibilitychange', onReturn)
    return () => {
      window.removeEventListener('focus', onReturn)
      document.removeEventListener('visibilitychange', onReturn)
    }
  }, [refresh])

  // The platforms are read at once; after a change to the local AI once more a little later (the model program and the ready files settle). A new call
  // restarts the wait, and the wait ends with the provider.
  const followUp = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(
    () => () => {
      if (followUp.current !== null) clearTimeout(followUp.current)
    },
    [],
  )
  const notifyChanged = useCallback<PlatformsContextValue['notifyChanged']>(
    (reason) => {
      void refresh()
      const delay = followUpDelay(reason)
      if (delay === null) return
      if (followUp.current !== null) clearTimeout(followUp.current)
      followUp.current = setTimeout(() => {
        followUp.current = null
        if (alive.current) void refresh()
      }, delay)
    },
    [refresh],
  )

  const value = useMemo(() => ({ platforms, error, version, refresh, notifyChanged }), [platforms, error, version, refresh, notifyChanged])
  return <PlatformsContext.Provider value={value}>{children}</PlatformsContext.Provider>
}
