import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchApplicationCountRaw } from '../../api'
import { stateFromAnswer, type ApplicationNumbersState } from './applicationNumbersRules'

// The numbers of every profile's applications, read once when the Profiles page opens (and again on `reload`, after a profile was added or deleted):
// one request for all profiles (plan 265). While the first read runs the state is 'loading'; a read that fails, or an answer without a usable list, makes it
// 'unknown' (the page then shows no numbers, never a wrong 0). A reload keeps showing the numbers it has until the new answer arrives; the latest read wins.
export function useApplicationNumbers(): { state: ApplicationNumbersState; reload: () => void } {
  const [state, setState] = useState<ApplicationNumbersState>({ status: 'loading' })
  const latest = useRef(0)
  const alive = useRef(true)

  const reload = useCallback(() => {
    const mine = ++latest.current
    fetchApplicationCountRaw().then(
      (raw) => {
        if (alive.current && mine === latest.current) setState(stateFromAnswer(raw))
      },
      () => {
        if (alive.current && mine === latest.current) setState({ status: 'unknown' })
      },
    )
  }, [])

  useEffect(() => {
    alive.current = true
    reload()
    return () => {
      alive.current = false
    }
  }, [reload])

  return { state, reload }
}
