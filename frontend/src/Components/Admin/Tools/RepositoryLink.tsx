import { isOpenableLink } from './repositoryDisplay'

// A repository's address, shown as a link that opens in a new tab of the same
// browser. rel="noopener noreferrer" keeps the opened page from reaching back into
// the app (window.opener) and from receiving the app's address as the referrer.
// Only http(s) addresses become links; anything else is shown as plain text.
export function RepositoryLink({ url, className = '' }: { url: string; className?: string }) {
  const base = `break-all font-mono text-xs ${className}`
  if (!isOpenableLink(url)) return <span className={`${base} text-gray-500`}>{url}</span>
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      title="Open in a new tab"
      className={`${base} text-gray-500 underline decoration-gray-300 underline-offset-2 hover:text-gray-900 hover:decoration-gray-500`}
    >
      {url}
    </a>
  )
}
