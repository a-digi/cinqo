import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkBreaks from 'remark-breaks'

// react-markdown never renders raw HTML from its source (no
// rehype-raw plugin here) — its AST-based rendering has no
// dangerouslySetInnerHTML anywhere, so this stays XSS-safe by
// construction even though content comes from two untrusted sources:
// the user's own typed message, and the upstream AI model's response
// (never developer-authored, and not something cinqo controls the
// content of). See plan/ai/frontend/frontend/step-12-markdown-message-rendering.md.
const components: Components = {
  p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="mb-2 list-disc space-y-1 pl-5 last:mb-0">{children}</ul>,
  ol: ({ children }) => <ol className="mb-2 list-decimal space-y-1 pl-5 last:mb-0">{children}</ol>,
  h1: ({ children }) => <h1 className="mb-2 mt-3 text-base font-semibold first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-2 mt-3 text-sm font-semibold first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1 mt-2 text-sm font-semibold first:mt-0">{children}</h3>,
  blockquote: ({ children }) => (
    <blockquote className="mb-2 border-l-2 border-gray-300 pl-3 italic text-gray-600 last:mb-0">{children}</blockquote>
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
      <code className="rounded bg-gray-200 px-1 py-0.5 font-mono text-xs" {...rest}>
        {children}
      </code>
    )
  },
  table: ({ children }) => <table className="mb-2 w-full border-collapse text-xs last:mb-0">{children}</table>,
  th: ({ children }) => <th className="border border-gray-300 bg-gray-100 px-2 py-1 text-left font-medium">{children}</th>,
  td: ({ children }) => <td className="border border-gray-300 px-2 py-1">{children}</td>,
  a: ({ href, children }) => {
    // Scheme allowlist — React does not itself block a `javascript:`
    // href, so anything other than http(s)/mailto renders as inert
    // plain text instead of a clickable link.
    const safe = typeof href === 'string' && /^(https?:|mailto:)/i.test(href)
    return safe ? (
      <a href={href} target="_blank" rel="noopener noreferrer" className="underline">
        {children}
      </a>
    ) : (
      <span>{children}</span>
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
