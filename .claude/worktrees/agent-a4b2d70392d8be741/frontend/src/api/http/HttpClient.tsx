import type { ReactNode } from 'react'
import { apiDelete, apiGet, apiPatch, apiPost, apiPut, apiUpload } from '../client'
import { HttpClientContext } from './HttpClientContext'

// Stubbed as a pure passthrough over client.ts's functions, on purpose —
// per this step's own design doc: "Not needed until cinqo has a real
// domain with more than one consumer of the same endpoint... stub it as
// a pure passthrough at skeleton stage, and add the dedup window when a
// real duplicate-request case shows up." coco-mda's own HttpClientProvider
// adds two things once there's a real reason to: a 100ms in-flight-request
// dedup window (keyed by method+endpoint+body), and global 5xx/network
// error surfacing via ErrorAlertContext (frontend/step-05, not built yet
// at this stage — this provider intentionally has no dependency on it).
export function HttpClientProvider({ children }: { children: ReactNode }) {
  return (
    <HttpClientContext.Provider
      value={{
        get: apiGet,
        post: apiPost,
        put: apiPut,
        patch: apiPatch,
        del: apiDelete,
        upload: apiUpload,
      }}
    >
      {children}
    </HttpClientContext.Provider>
  )
}
