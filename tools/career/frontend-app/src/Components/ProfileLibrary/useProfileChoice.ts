import { useEffect, useMemo, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../api'
import { useAddressSearch } from '../../Shared/AddressState/useAddressSearch'
import { writeAddress } from '../../Shared/AddressState/writeAddress'
import { getProfileIdFromLocalStorage, setProfileIdInLocalStorage } from '../ProfileSwitcher/repositoryLocalStorage'
import { buildProfileSearch, chooseProfileId, parseProfileParam } from './libraryRules'

// The profile a library page (Skills, Experience) shows: it comes from the address (?profileId=), else the profile last chosen in this
// browser, else the first one; choosing another writes the address (Back returns to the earlier one) and is remembered.
export function useProfileChoice(ownPath: string) {
  const address = useAddressSearch(ownPath)
  const fromAddress = useMemo(() => parseProfileParam(address.search), [address.search])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchProfiles(), fetchPersonas()])
      .then(([profileList, personaList]) => {
        setProfiles(profileList)
        setPersonas(personaList)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  const profileId = chooseProfileId(profiles, fromAddress, getProfileIdFromLocalStorage())

  function select(id: string) {
    setProfileIdInLocalStorage(id)
    writeAddress(window, ownPath, buildProfileSearch(id), 'push')
  }

  return { profiles, personas, profileId, select, error, loaded }
}
