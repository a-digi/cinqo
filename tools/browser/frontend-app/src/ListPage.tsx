import { useEffect, useState } from 'react'
import { fetchCredentials, removeCredential, type CredentialSummary } from './api'
import { PlusIcon } from './icons'

const ADD_PATH = '/tools/browser/credentials/new'

export function ListPage() {
  const [credentials, setCredentials] = useState<CredentialSummary[]>([])
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchCredentials()
      .then(setCredentials)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
  }, [])

  function handleRemove(domain: string) {
    setError('')
    removeCredential(domain)
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl py-6 font-sans text-gray-900">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="mb-1.5 text-xl font-semibold">Login Credentials</h1>
          <p className="mb-5 text-sm text-gray-500">
            Domains the Browser tool can log into on your behalf. The AI can ask whether a
            credential exists for a domain, but never sees the username or password stored here.
          </p>
        </div>
        <button
          type="button"
          aria-label="Add credential"
          title="Add credential"
          onClick={() => window.__cinqoToolBridge.navigate(ADD_PATH)}
          className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          Add
        </button>
      </div>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="overflow-hidden rounded-md border border-gray-200 shadow-sm">
        <table className="w-full text-sm">
          <thead>
            <tr>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Domain</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Username</th>
              <th className="border-b border-gray-200 bg-gray-50 p-3"></th>
            </tr>
          </thead>
          <tbody>
            {credentials.map((cred) => (
              <tr key={cred.domain} className="last:[&>td]:border-b-0 hover:bg-gray-50">
                <td className="border-b border-gray-200 p-3">{cred.domain}</td>
                <td className="border-b border-gray-200 p-3">{cred.username}</td>
                <td className="border-b border-gray-200 p-3">
                  <button
                    type="button"
                    onClick={() => handleRemove(cred.domain)}
                    className="rounded-md border border-gray-200 px-3 py-1 text-red-700 transition-colors hover:bg-red-50"
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
