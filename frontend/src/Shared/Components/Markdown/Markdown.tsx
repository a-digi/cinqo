import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkBreaks from 'remark-breaks'

// Extracts the generated file's id from a tool proxy download href and
// turns it into a real save-as filename — the one part of that href
// this component already trusts (it's what made the href "safe" to
// render as a link in the first place), unlike the link's own visible
// text. See plan/ai/tools/pdf-generator/step-07-download-link-fix.md.
function proxyDownloadFilename(href: string): string | null {
  const match = /^\/api\/v1\/tools\/[^/]+\/proxy\/files\?id=([^&]+)$/.exec(href)
  return match ? `${decodeURIComponent(match[1])}.pdf` : null
}

// react-markdown never renders raw HTML from its source (no
// rehype-raw plugin here) — its AST-based rendering has no
// dangerouslySetInnerHTML anywhere, so this stays XSS-safe by
// construction even though content comes from two untrusted sources:
// the user's own typed message, and the upstream AI model's response
// (never developer-authored, and not something cinqo controls the
// content of). See plan/ai/frontend/frontend/step-12-markdown-message-rendering.md.
// break-words (overflow-wrap: break-word) on every text-bearing
// element below — an unbroken long token (a long URL, file path, or
// base64 blob) in a user prompt or an AI reply otherwise overflows the
// floating chat widget's own narrow (380px) container horizontally
// instead of wrapping, since none of react-markdown's default element
// renderings set it on their own. Deliberately NOT applied to the
// fenced code block below (pre/its own block code) — that one already
// has its own considered "preserve formatting, scroll horizontally
// instead" behavior (overflow-x-auto), which this would undermine; the
// same reasoning already established for the sibling failed-message
// error <pre> in MessageThread.tsx's own Bubble. See
// plan/ai/frontend/frontend/step-XX-chat-widget-long-message-wrapping.md.
const components: Components = {
  p: ({ children }) => <p className="mb-2 break-words last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="mb-2 list-disc space-y-1 break-words pl-5 last:mb-0">{children}</ul>,
  ol: ({ children }) => <ol className="mb-2 list-decimal space-y-1 break-words pl-5 last:mb-0">{children}</ol>,
  h1: ({ children }) => <h1 className="mb-2 mt-3 break-words text-base font-semibold first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-2 mt-3 break-words text-sm font-semibold first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1 mt-2 break-words text-sm font-semibold first:mt-0">{children}</h3>,
  blockquote: ({ children }) => (
    <blockquote className="mb-2 border-l-2 border-gray-300 pl-3 italic text-gray-600 last:mb-0 break-words">{children}</blockquote>
  ),
  pre: ({ children }) => <pre className="mb-2 overflow-x-auto rounded bg-gray-800 p-2 last:mb-0">{children}</pre>,
  code({ className, children, ...rest }) {
    // react-markdown's own documented pattern for telling a fenced
    // code block apart from an inline code span (v9+ no longer passes
    // an `inline` prop) — only a ```-fenced block's <code> carries a
    // `language-*` className.
    const isBlock = /language-(\w+)/.exec(className ?? '')
    return isBlock ? (
      <code className="block font-mono text-xs text-gray-100" {...rest}>
        {children}
      </code>
    ) : (
      // Inline code (not a fenced block) DOES get break-words, unlike
      // the block case above — a long inline token (e.g. a path)
      // should wrap within the bubble rather than push it wide.
      <code className="break-words rounded bg-gray-200 px-1 py-0.5 font-mono text-xs" {...rest}>
        {children}
      </code>
    )
  },
  // A table's own layout can still force per-column min-content widths
  // wider than the narrow chat widget even with break-words on each
  // cell (e.g. several columns of short-but-numerous content) — wrapped
  // in its own horizontal scroller, same as the fenced code block
  // above, rather than letting it push the whole bubble wide.
  table: ({ children }) => (
    <div className="mb-2 overflow-x-auto last:mb-0">
      <table className="w-full border-collapse text-xs">{children}</table>
    </div>
  ),
  th: ({ children }) => <th className="break-words border border-gray-300 bg-gray-100 px-2 py-1 text-left font-medium">{children}</th>,
  td: ({ children }) => <td className="break-words border border-gray-300 px-2 py-1">{children}</td>,
  a: ({ href, children }) => {
    // Scheme allowlist — React does not itself block a `javascript:`
    // href, so anything other than http(s)/mailto renders as inert
    // plain text instead of a clickable link. The one relative shape
    // admitted alongside that is the tool-download link
    // appendResourceLinks (api/src/conversation/chat.go) deterministically
    // generates itself — not arbitrary relative text a model could
    // produce — anchored at the start so a protocol-relative `//host/...`
    // (which also starts with "/") can't slip through. See
    // plan/ai/tools/pdf-generator/step-07-download-link-fix.md.
    const isProxyDownload = typeof href === 'string' && /^\/api\/v1\/tools\/[^/]+\/proxy\//.test(href)
    const safe = typeof href === 'string' && (/^(https?:|mailto:)/i.test(href) || isProxyDownload)
    if (!safe) {
      return <span>{children}</span>
    }

    // An empty `download` attribute leaves the actual saved filename up
    // to browser-specific fallback behavior instead of reliably
    // deferring to the response's own Content-Disposition — in
    // practice this landed on the URL's last path segment ("files",
    // since the real id only ever lives in the query string), not the
    // real filename. Derived here from the id already inside the
    // href — not from `children` (the model-controlled visible link
    // text) — since a hallucinated label could otherwise pair a
    // legitimate href with a misleading save-as name. See this file's
    // step-07 doc, "Amendment" section.
    const filename = isProxyDownload ? proxyDownloadFilename(href) : null

    return (
      <a
        href={href}
        download={filename ?? undefined}
        target={isProxyDownload ? undefined : '_blank'}
        rel="noopener noreferrer"
        className="underline"
      >
        {children}
      </a>
    )
  },
}

export function Markdown({ content }: { content: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} components={components}>
      {content}
    </ReactMarkdown>
  )
}
