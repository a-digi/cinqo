import { useEffect, useState } from 'react'
import { fetchSecurityScopes, type SecurityScopes } from '../../../api/security'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { referencedScopeIds } from '../../../config/security/scopeConformance'

// Admin-only view of the scope registry (api/src/security/scopes) — see
// plan/ai/security/security.md. Read-only: scopes are declared in code,
// not editable from here.
export function ScopesPage() {
  const [data, setData] = useState<SecurityScopes | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetchSecurityScopes()
      .then((result) => {
        if (!cancelled) setData(result)
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load scopes.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) {
    return <p className="p-6 text-sm text-red-600">{error}</p>
  }

  if (!data) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading scopes…" />
      </div>
    )
  }

  const registeredIds = new Set(data.groups.flatMap((g) => g.scopes.map((s) => s.id)))
  const unknownFrontendScopes = referencedScopeIds().filter((id) => !registeredIds.has(id))

  return (
    <div className="max-w-4xl space-y-8 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Scopes</h1>
        <p className="mt-1 text-sm text-gray-500">
          Every scope this application (and its plugins) defines, and whether it's actually enforced by a route and
          requested from the identity provider.
        </p>
      </div>

      {data.unregisteredEnforcedScopes.length > 0 && (
        <DiagnosticBanner
          title="Enforced but never registered"
          description="A route checks these scopes, but no domain declared them — no description exists for whoever assigns them."
          scopes={data.unregisteredEnforcedScopes}
        />
      )}

      {data.unrequestedScopes.length > 0 && (
        <DiagnosticBanner
          title="Not requested from the identity provider"
          description="These scopes can never be granted to any user — they're missing from the OAuth scopes requested at login."
          scopes={data.unrequestedScopes}
        />
      )}

      {unknownFrontendScopes.length > 0 && (
        <DiagnosticBanner
          title="Referenced by the frontend but not a registered backend scope"
          description="These scope strings appear in the frontend's own AppScopes catalog but no longer match anything the backend registers — likely renamed or removed. Any menu entry or guard using one of these stays silently hidden/denied."
          scopes={unknownFrontendScopes}
        />
      )}

      <div className="space-y-6">
        {data.groups.map((group) => (
          <div key={group.id} className="rounded-lg border border-gray-200">
            <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
              <h2 className="font-mono text-sm font-semibold text-gray-900">{group.id}</h2>
              <p className="mt-0.5 text-sm text-gray-500">{group.description}</p>
            </div>
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-4 py-2 font-medium">Scope</th>
                  <th className="px-4 py-2 font-medium">Description</th>
                  <th className="px-4 py-2 font-medium">Enforced</th>
                  <th className="px-4 py-2 font-medium">Requested</th>
                </tr>
              </thead>
              <tbody>
                {group.scopes.map((scope) => (
                  <tr key={scope.id} className="border-t border-gray-100">
                    <td className="px-4 py-2 font-mono text-xs text-gray-700">{scope.id}</td>
                    <td className="px-4 py-2 text-gray-600">{scope.description}</td>
                    <td className="px-4 py-2">
                      <Badge ok={scope.enforced} />
                    </td>
                    <td className="px-4 py-2">
                      <Badge ok={scope.requested} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      </div>
    </div>
  )
}

function Badge({ ok }: { ok: boolean }) {
  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
        ok ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
      }`}
    >
      {ok ? 'Yes' : 'No'}
    </span>
  )
}

function DiagnosticBanner({ title, description, scopes }: { title: string; description: string; scopes: string[] }) {
  return (
    <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
      <h3 className="text-sm font-semibold text-amber-800">{title}</h3>
      <p className="mt-0.5 text-sm text-amber-700">{description}</p>
      <ul className="mt-2 space-y-0.5">
        {scopes.map((s) => (
          <li key={s} className="font-mono text-xs text-amber-900">
            {s}
          </li>
        ))}
      </ul>
    </div>
  )
}
