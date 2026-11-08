import type { Publisher, ToolMetadata } from '../../../api/toolMeta'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import {
  CLAIM_HINT,
  NOT_STATED,
  NOT_STATED_HINT,
  authorLinks,
  byLine,
  categoryLabel,
  generatedLabel,
  linkEntries,
  publisherLine,
} from './toolMetaDisplay'

const anchor = 'font-medium text-gray-700 underline decoration-gray-300 underline-offset-2 hover:text-gray-900 hover:decoration-gray-500'

// "by Ada +1 · Acme GmbH", or a quiet "Developer not stated". The text is a claim by the publisher, which the
// tooltip says.
export function Byline({ metadata, className = '' }: { metadata: ToolMetadata | null | undefined; className?: string }) {
  const line = byLine(metadata)
  if (line === null) {
    return (
      <p className={`text-xs italic text-gray-400 ${className}`} title={NOT_STATED_HINT}>
        {NOT_STATED}
      </p>
    )
  }
  return (
    <p className={`truncate text-xs text-gray-600 ${className}`} title={`${line}. ${CLAIM_HINT}`}>
      {line}
    </p>
  )
}

// The licence, as an outline chip like the others on a card; nothing when it is not stated.
export function LicensePill({ license }: { license: string | undefined }) {
  return license ? <Pill outline>{license}</Pill> : null
}

// Small text links (homepage, source, …) that show their host, so a look-alike address is visible. Only safe
// addresses are drawn at all; every one opens in a new tab without handing over the opener.
export function LinkRow({ links, className = '' }: { links: ToolMetadata['links']; className?: string }) {
  const entries = linkEntries(links)
  if (entries.length === 0) return null
  return (
    <p className={`flex flex-wrap gap-x-3 gap-y-0.5 text-xs ${className}`}>
      {entries.map((e) => (
        <a
          key={e.key}
          href={e.url}
          {...(e.mail ? {} : { target: '_blank', rel: 'noopener noreferrer' })}
          title={`${e.label}: ${e.host}`}
          className={anchor}
        >
          {e.label}
        </a>
      ))}
    </p>
  )
}

// One quiet line for a table row: "Maintained by Cinqo team · Acme", or "Maintainer not stated".
export function MaintainerLine({ publisher }: { publisher: Publisher | null | undefined }) {
  const line = publisherLine(publisher)
  return line === null ? (
    <p className="text-xs italic text-gray-400" title={NOT_STATED_HINT}>
      Maintainer not stated
    </p>
  ) : (
    <p className="truncate text-xs text-gray-500" title={CLAIM_HINT}>
      {line}
    </p>
  )
}

// Who maintains a repository, under its name: the maintainers, their company, and where to find them.
export function PublisherBlock({ publisher }: { publisher: Publisher | null | undefined }) {
  const line = publisherLine(publisher)
  if (line === null || !publisher) {
    return (
      <p className="mt-1 text-xs italic text-gray-400" title={NOT_STATED_HINT}>
        Maintainer not stated
      </p>
    )
  }
  const updated = generatedLabel(publisher.generatedAt)
  return (
    <div className="mt-1 space-y-0.5">
      <p className="text-xs text-gray-600" title={CLAIM_HINT}>
        {line}
        {updated && <span className="text-gray-400"> · updated {updated}</span>}
      </p>
      <LinkRow links={publisher.links} />
    </div>
  )
}

// Everything the publisher states about a tool, for the dialog shown before an install or update: each author
// (with their address if they gave one), the company, the licence, the links with their hosts, the copyright. It
// ends by saying what it is: a claim.
export function ToolFacts({ metadata }: { metadata: ToolMetadata | null | undefined }) {
  const authors = metadata?.authors ?? []
  if (!metadata || authors.length === 0) {
    return (
      <div className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">
        <p className="font-medium">{NOT_STATED}</p>
        <p className="mt-0.5">This repository does not say who made this tool. Only continue if you trust the repository.</p>
      </div>
    )
  }
  const category = categoryLabel(metadata.category)
  return (
    <div className="space-y-2 rounded-md bg-gray-50 px-3 py-2 text-xs text-gray-700">
      <dl className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 gap-y-1">
        <dt className="text-gray-500">{authors.length === 1 ? 'Author' : 'Authors'}</dt>
        <dd className="space-y-0.5">
          {authors.map((a, i) => {
            const links = authorLinks(a)
            return (
              <div key={i}>
                <span className="font-medium text-gray-900">{a.name}</span>
                {links.email && (
                  <>
                    {' '}
                    <a href={`mailto:${links.email}`} className={anchor}>
                      {links.email}
                    </a>
                  </>
                )}
                {links.url && (
                  <>
                    {' '}
                    <a href={links.url.href} target="_blank" rel="noopener noreferrer" className={anchor}>
                      {links.url.host}
                    </a>
                  </>
                )}
              </div>
            )
          })}
        </dd>
        {metadata.company && (
          <>
            <dt className="text-gray-500">Company</dt>
            <dd>{metadata.company.name}</dd>
          </>
        )}
        {metadata.license && (
          <>
            <dt className="text-gray-500">License</dt>
            <dd>{metadata.license}</dd>
          </>
        )}
        {category && (
          <>
            <dt className="text-gray-500">Category</dt>
            <dd>{category}</dd>
          </>
        )}
        {linkEntries(metadata.links).map((e) => (
          <FactLink key={e.key} label={e.label} url={e.url} host={e.host} mail={e.mail} />
        ))}
        {metadata.copyright && (
          <>
            <dt className="text-gray-500">Copyright</dt>
            <dd>{metadata.copyright}</dd>
          </>
        )}
      </dl>
      <p className="text-gray-500">{CLAIM_HINT}</p>
    </div>
  )
}

function FactLink({ label, url, host, mail }: { label: string; url: string; host: string; mail: boolean }) {
  return (
    <>
      <dt className="text-gray-500">{label}</dt>
      <dd className="truncate">
        <a href={url} {...(mail ? {} : { target: '_blank', rel: 'noopener noreferrer' })} className={anchor}>
          {host}
        </a>
      </dd>
    </>
  )
}
