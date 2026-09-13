import { useAuth } from './AuthContext'

// No form fields — cinqo has no password to collect. login() (auth/01)
// does the entire PKCE + redirect dance; this component only needs to
// trigger it.
export function LoginPage() {
  const { login } = useAuth()
  return (
    <div className="flex min-h-screen items-center justify-center">
      <button
        type="button"
        onClick={() => void login()}
        className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800"
      >
        Sign in
      </button>
    </div>
  )
}
