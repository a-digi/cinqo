import { useState, type FormEvent } from 'react'
import { normalizeDomain, saveCredential } from './api'

const LIST_PATH = '/tools/browser/credentials'

export function AddPage() {
  const [domain, setDomain] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  function handleSubmit(event: FormEvent) {
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
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl py-6 font-sans text-gray-900">
      <a
        href="#"
        onClick={(e) => {
          e.preventDefault()
          window.__cinqoToolBridge.navigate(LIST_PATH)
        }}
        className="mb-4 inline-block text-sm text-gray-500 hover:underline"
      >
        &larr; Back to Login Credentials
      </a>

      <h1 className="mb-1.5 text-xl font-semibold">Add / Update Credential</h1>
      <p className="mb-5 text-sm text-gray-500">
        Saving an existing domain again updates its stored credential. The AI never sees the
        username or password entered here.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <form
        onSubmit={handleSubmit}
        className="rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm"
      >
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
            onChange={(e) => setDomain(e.target.value)}
            className="w-full rounded-md border border-gray-200 bg-white px-3 py-2.5 text-sm transition-shadow focus:border-gray-900 focus:outline-none focus:ring-3 focus:ring-gray-900/10"
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
            onChange={(e) => setUsername(e.target.value)}
            className="w-full rounded-md border border-gray-200 bg-white px-3 py-2.5 text-sm transition-shadow focus:border-gray-900 focus:outline-none focus:ring-3 focus:ring-gray-900/10"
          />
        </div>
        <div className="mb-4">
          <label htmlFor="cinqo-browser-cred-password" className="mb-1.5 block text-sm font-medium">
            Password
          </label>
          <input
            id="cinqo-browser-cred-password"
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded-md border border-gray-200 bg-white px-3 py-2.5 text-sm transition-shadow focus:border-gray-900 focus:outline-none focus:ring-3 focus:ring-gray-900/10"
          />
        </div>
        <button
          type="submit"
          className="rounded-md bg-gray-900 px-4.5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          Save
        </button>
      </form>
    </div>
  )
}
