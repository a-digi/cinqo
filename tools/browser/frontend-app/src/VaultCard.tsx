import { LockIcon } from './icons'
import { ACCENTS, PAGE_ACCENT, accentFor } from './Shared/theme'
import type { SuffixCount } from './credentialRules'

// The Login page's overview: a card on the left that says how many logins are saved, what
// kinds of site they are for (the domain suffix: .com 4, .de 2 …), and what the AI can and
// cannot see of them.
export function VaultCard({ total, suffixes }: { total: number; suffixes: SuffixCount[] }) {
  return (
    <section className="browser-enter browser-delay-1 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="flex items-center gap-3">
        <span
          className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white"
          style={{ background: `linear-gradient(135deg, ${ACCENTS[4].from}, ${ACCENTS[4].to})` }}
        >
          <LockIcon />
        </span>
        <div>
          <div className="text-2xl font-bold leading-none text-gray-900">{total}</div>
          <div className="mt-1 text-xs text-gray-500">{total === 1 ? 'saved login' : 'saved logins'}</div>
        </div>
      </div>

      {suffixes.length > 0 && (
        <>
          <div className="mb-1.5 mt-4 text-xs font-semibold uppercase tracking-wide text-gray-400">By site type</div>
          <div className="flex flex-wrap gap-1.5">
            {suffixes.map((s) => {
              const accent = accentFor(s.suffix)
              return (
                <span
                  key={s.suffix}
                  className="rounded-full px-2.5 py-0.5 text-xs font-medium"
                  style={{ background: accent.soft, color: accent.text }}
                >
                  {s.suffix} <span className="font-bold">{s.count}</span>
                </span>
              )
            })}
          </div>
        </>
      )}

      <p className="mt-4 border-t pt-3 text-xs text-gray-500" style={{ borderColor: PAGE_ACCENT.soft }}>
        The AI can ask whether a login exists for a site, but never sees the username or the password.
      </p>
    </section>
  )
}
