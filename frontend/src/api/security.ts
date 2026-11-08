import { apiGet } from './client'

// The backend (api/src/security/scopes/handler/scopes_handler.go) returns
// snake_case JSON — same *Raw-to-camelCase mapping convention as
// frontend/src/api/auth.ts.

export interface ScopeView {
  id: string
  description: string
  enforced: boolean
  requested: boolean
}

export interface ScopeGroupView {
  id: string
  description: string
  scopes: ScopeView[]
}

export interface SecurityScopes {
  groups: ScopeGroupView[]
  unregisteredEnforcedScopes: string[]
  unrequestedScopes: string[]
}

interface SecurityScopesRaw {
  groups: ScopeGroupView[]
  unregistered_enforced_scopes: string[]
  unrequested_scopes: string[]
}

export async function fetchSecurityScopes(): Promise<SecurityScopes> {
  const raw = await apiGet<SecurityScopesRaw>('/api/v1/security/scopes')
  return {
    groups: raw.groups,
    unregisteredEnforcedScopes: raw.unregistered_enforced_scopes,
    unrequestedScopes: raw.unrequested_scopes,
  }
}
