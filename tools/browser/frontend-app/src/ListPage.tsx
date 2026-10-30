import { useEffect, useState } from 'react'
import { fetchCredentials, removeCredential, type CredentialSummary } from './api'
import { KeyIcon, PlusIcon } from './icons'
import { suffixCounts } from './credentialRules'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { plural } from './Shared/plural'
import { PAGE_ACCENT, accentFor, initialOf } from './Shared/theme'
import { VaultCard } from './VaultCard'

const ADD_PATH = '/tools/browser/credentials/new'

// The saved logins: a vault card with the overview on the left, one card per login on the
// right. Only the domain and the username are ever read here; the password is write-only.
export function ListPage() {
  const [credentials, setCredentials] = useState<CredentialSummary[]>([])
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchCredentials()
      .then(setCredentials)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
  }, [])

  function handleRemove(domain: string) {
    setError('')
    removeCredential(domain)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="browser-v-left browser-a-sheen browser-t-wave max-w-6xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<KeyIcon />}
        title="Login Credentials"
        description="Domains the Browser tool can log into on your behalf. The AI can ask whether a credential exists for a domain, but never sees the username or password stored here."
        pill={{ text: plural(credentials.length, 'login', 'logins') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        {/* No w-full here: every tool's CSS is injected into the same page, and another tool's .w-full
            can land after this lg:w-* rule and win, stretching the aside across the whole row. */}
        <aside className="shrink-0 lg:sticky lg:top-6 lg:w-60">
          <VaultCard total={credentials.length} suffixes={suffixCounts(credentials)} />
        </aside>

        <div className="min-w-0 flex-1">
          <div className="mb-3 flex items-center justify-between gap-3">
            <p className="text-xs text-gray-500">Adding a login for a domain that already has one replaces it.</p>
            <button
              type="button"
              aria-label="Add credential"
              title="Add credential"
              onClick={() => {
                window.__cinqoToolBridge.navigate(ADD_PATH)
              }}
              className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
            >
              <PlusIcon />
              Add login
            </button>
          </div>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {credentials.length === 0 && !error && (
            <div
              className="browser-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No logins yet. Add the first one with “Add login”.
            </div>
          )}

          <div className="browser-stagger space-y-3">
            {credentials.map((cred) => (
              <div
                key={cred.domain}
                className="flex items-center gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
              >
                <span
                  className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                  style={{ background: accentFor(cred.domain).to }}
                  aria-hidden="true"
                >
                  {initialOf(cred.domain)}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-semibold">{cred.domain}</div>
                  <div className="truncate text-xs text-gray-500">{cred.username}</div>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    handleRemove(cred.domain)
                  }}
                  className="shrink-0 rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
