import { useEffect, useRef, useState } from 'react'
import { profileToFocus } from './profileRules'

// How long the card of the profile the address names stays highlighted.
export const FOCUS_MS = 2500

// The Profiles page's "show me this profile" (plan 233): when the address names a profile (?profileId=, the back link of its pages) the card
// `profile-<id>` is scrolled into view, once, as soon as the profiles are loaded, and the profile's id is returned for FOCUS_MS so the page can
// highlight the card. A later reload of the list (adding a link) does not scroll again.
export function useFocusedProfile(profiles: { id: string }[]): string | null {
  const [focusId, setFocusId] = useState<string | null>(null)
  const done = useRef(false)

  useEffect(() => {
    if (done.current || profiles.length === 0) return
    done.current = true
    const id = profileToFocus(window.location.search, profiles)
    if (id === null) return
    document.getElementById(`profile-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setFocusId(id)
    const timer = window.setTimeout(() => {
      setFocusId(null)
    }, FOCUS_MS)
    return () => {
      window.clearTimeout(timer)
    }
  }, [profiles])

  return focusId
}
