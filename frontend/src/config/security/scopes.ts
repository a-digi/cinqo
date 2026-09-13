// Frontend-side mirror of the backend's scope catalog
// (plan/ai/backend/auth/step-04-scope-naming-convention.md). Kept in sync
// BY HAND with the backend's config/scopes/*.json — there is no shared
// source of truth between frontend and backend for this, matching every
// sibling coco-aim app.
//
// Started in auth/02 (AuthGuard needs the super-admin bypass) with just
// SuperAdmin; the Ping entries were added here in auth/04. Extend this
// object with each new domain's own scopes as real features land.
export const AppScopes = {
  SuperAdmin: 'cinqo:super:admin',
  PingRead: 'cinqo:ping:read',
  PingCreate: 'cinqo:ping:create',
} as const
