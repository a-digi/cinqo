import { useEffect, useState, type SyntheticEvent } from 'react'
import { fetchCredentials, normalizeDomain, saveCredential, type CredentialSummary } from './api'
import { credentialPlan } from './credentialRules'
import { KeyIcon } from './icons'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { inputClass } from './Shared/inputClass'
import { WillHappenCard } from './WillHappenCard'

const LIST_PATH = '/tools/browser/credentials'

// The page behind the Login list's "Add login": the form, and beside it a card that says
// what saving will do. The card compares the typed domain with the saved logins (domain and
// username only, the same list the Login page reads); no password is ever read or shown.
export function AddPage() {
  const [domain, setDomain] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [saved, setSaved] = useState<CredentialSummary[] | null>(null)

  useEffect(() => {
    fetchCredentials()
      .then(setSaved)
      .catch(() => {
        // Advice only: the form works without it, the card just cannot compare.
        setSaved(null)
      })
  }, [])

  function handleSubmit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')

    saveCredential(normalizeDomain(domain.trim()), username, password)
      .then(() => {
        // Clear the form's own state explicitly, not just relying on
        // the DOM's default post-submit state — the plaintext
        // password shouldn't linger any longer than necessary. Then
        // go back to the list so the user immediately sees the
        // new/updated row.
        setDomain('')
        setUsername('')
        setPassword('')
        window.__cinqoToolBridge.navigate(LIST_PATH)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const plan = credentialPlan(normalizeDomain(domain.trim()), saved ?? [])

  return (
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <button
        type="button"
        onClick={() => {
          window.__cinqoToolBridge.navigate(LIST_PATH)
        }}
        className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
      >
        &larr; Back to Login Credentials
      </button>

      <PageHeaderCard
        icon={<KeyIcon />}
        title="Add / Update Credential"
        description="Saving an existing domain again updates its stored credential. The AI never sees the username or password entered here."
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          <form onSubmit={handleSubmit} className={`browser-enter browser-delay-1 ${cardClassName}`} style={cardStyle}>
            <div className="mb-4">
              <label htmlFor="cinqo-browser-cred-domain" className="mb-1.5 block text-sm font-medium">
                Domain
              </label>
              <input
                id="cinqo-browser-cred-domain"
                type="text"
                placeholder="example.com"
                required
                value={domain}
                onChange={(e) => {
                  setDomain(e.target.value)
                }}
                className={inputClass}
              />
            </div>
            <div className="mb-4">
              <label htmlFor="cinqo-browser-cred-username" className="mb-1.5 block text-sm font-medium">
                Username
              </label>
              <input
                id="cinqo-browser-cred-username"
                type="text"
                required
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value)
                }}
                className={inputClass}
              />
            </div>
            <div className="mb-5">
              <label htmlFor="cinqo-browser-cred-password" className="mb-1.5 block text-sm font-medium">
                Password
              </label>
              <input
                id="cinqo-browser-cred-password"
                type="password"
                required
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value)
                }}
                className={inputClass}
              />
            </div>
            <div className="flex gap-2">
              <button
                type="submit"
                className="rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
              >
                {plan.kind === 'replace' ? 'Replace login' : 'Save'}
              </button>
              <button
                type="button"
                onClick={() => {
                  window.__cinqoToolBridge.navigate(LIST_PATH)
                }}
                className="rounded-md border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </form>
        </div>

        <aside className="shrink-0 lg:sticky lg:top-6 lg:w-72">
          <WillHappenCard plan={plan} listKnown={saved !== null} />
        </aside>
      </div>
    </div>
  )
}
