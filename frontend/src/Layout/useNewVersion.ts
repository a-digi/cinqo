import { useEffect, useState } from 'react'
import { VERSION_CHECK_MS, isNewBuild, signatureOfDocument, signatureOfHtml } from './appVersion'

// true once the server serves a different build than this window runs. It asks for index.html (which the server always revalidates) every minute
// while the tab is visible and as soon as the window gets focus or the tab is shown, and stops asking once it has found a new build. A failed
// request (the app is restarting) is not an answer: nothing is shown and it tries again later.
export function useNewVersion(intervalMs: number = VERSION_CHECK_MS): boolean {
  const [available, setAvailable] = useState(false)

  useEffect(() => {
    const running = signatureOfDocument(document)
    if (running === '') return // not a built app (a dev server): nothing to compare
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let inFlight = false

    const check = async () => {
      if (stopped || inFlight || document.visibilityState === 'hidden') return
      inFlight = true
      try {
        const res = await fetch('/index.html', { cache: 'no-store', credentials: 'same-origin' })
        if (res.ok && isNewBuild(running, signatureOfHtml(await res.text()))) {
          stopped = true
          setAvailable(true)
          return
        }
      } catch {
        // the app is restarting or the network is down: try again later
      } finally {
        inFlight = false
      }
      schedule()
    }
    const schedule = () => {
      if (!stopped) timer = setTimeout(() => void check(), intervalMs)
    }
    const nowOnWake = () => {
      if (document.visibilityState === 'hidden') return
      clearTimeout(timer)
      void check()
    }

    schedule()
    window.addEventListener('focus', nowOnWake)
    document.addEventListener('visibilitychange', nowOnWake)
    return () => {
      stopped = true
      clearTimeout(timer)
      window.removeEventListener('focus', nowOnWake)
      document.removeEventListener('visibilitychange', nowOnWake)
    }
  }, [intervalMs])

  return available
}
