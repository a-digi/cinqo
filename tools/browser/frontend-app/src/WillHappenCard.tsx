import type { CredentialPlan } from './credentialRules'
import { PAGE_ACCENT } from './Shared/theme'

// The saved logins list can come back with the username masked (a row of bullets); that says
// nothing, so it is left out of the sentence.
function hasReadableUsername(username: string): boolean {
  return /[^•\s*]/.test(username)
}

// The Add / Update Credential page's overview: a card beside the form that says, as the
// domain is typed, what saving will do (add a new login, or replace the one that is saved,
// with its current username but never a password), and what the form guarantees.
export function WillHappenCard({ plan, listKnown }: { plan: CredentialPlan; listKnown: boolean }) {
  const tone =
    plan.kind === 'add'
      ? { box: 'border-green-200 bg-green-50 text-green-800', mark: '+', title: 'New login' }
      : plan.kind === 'replace'
        ? { box: 'border-amber-300 bg-amber-50 text-amber-900', mark: '↻', title: 'Replaces a login' }
        : plan.kind === 'invalid'
          ? { box: 'border-red-200 bg-red-50 text-red-800', mark: '!', title: 'Check the domain' }
          : { box: 'border-gray-200 bg-gray-50 text-gray-600', mark: '…', title: 'Waiting for a domain' }

  return (
    <section className="rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <h2 className="mb-3 text-sm font-semibold text-gray-800">What will happen</h2>

      <div className={`rounded-2xl border p-3 text-sm ${tone.box}`} role="status">
        <div className="mb-0.5 flex items-center gap-2 font-semibold">
          <span aria-hidden="true" className="flex h-5 w-5 items-center justify-center rounded-full bg-white/70 text-xs">
            {tone.mark}
          </span>
          {tone.title}
        </div>
        <p className="text-xs">
          {plan.kind === 'empty' && 'Type a domain to see whether it already has a login.'}
          {plan.kind === 'invalid' && 'A domain is a host name such as example.com, without spaces or a path.'}
          {plan.kind === 'add' && (
            <>
              Adds a login for <span className="font-mono font-medium">{plan.domain}</span>.
              {!listKnown && ' The saved logins could not be read, so it cannot say whether this replaces one.'}
            </>
          )}
          {plan.kind === 'replace' && (
            <>
              Replaces the login for <span className="font-mono font-medium">{plan.domain}</span>
              {hasReadableUsername(plan.currentUsername) && (
                <>
                  , now saved for <span className="font-medium">{plan.currentUsername}</span>
                </>
              )}
              . The stored username and password are overwritten.
            </>
          )}
        </p>
      </div>

      <ul className="mt-4 space-y-2 text-xs text-gray-600">
        {[
          'Stored encrypted by the Browser tool, on the computer where Cinqo runs.',
          'The AI can ask whether a login exists for a site, but never sees the username or password.',
          'The password cannot be read back here; to change it, save the domain again.',
        ].map((line) => (
          <li key={line} className="flex gap-2">
            <span aria-hidden="true" className="mt-0.5 text-green-600">
              ✓
            </span>
            {line}
          </li>
        ))}
      </ul>
    </section>
  )
}
