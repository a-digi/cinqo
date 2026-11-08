// AuthConfig/AuthMeResponse (the wire-shape response types) live in
// ../../api/auth.ts, not here — see frontend/step-03's note on keeping
// the dependency direction the usual way round (UI imports from api/).
// AuthUser is the one shape genuinely local to the auth UI layer: the
// subset of AuthMeResponse that's actually useful to render.

export interface AuthUser {
  userId: string
  email?: string
  name?: string
  preferredUsername?: string
}
