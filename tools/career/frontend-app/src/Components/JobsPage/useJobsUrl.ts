import { useMemo } from 'react'
import { useAddressSearch } from '../../Shared/AddressState/useAddressSearch'
import { parseJobsUrl, type JobsListState } from './jobsUrlState'

// The Jobs list's state as the address says it (step 135): the shared address hook plus the Jobs rules.
export function useJobsUrl(ownPath: string): { state: JobsListState; tick: number } {
  const { search, tick } = useAddressSearch(ownPath)
  const state = useMemo(() => parseJobsUrl(search), [search])
  return { state, tick }
}
