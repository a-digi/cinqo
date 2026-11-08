// A link to another page of the app: a real <a>, so it can be opened in a new tab, whose plain click navigates
// inside the app instead of reloading it. A modified click (new tab, new window) keeps the link's own behaviour.
export function AppLink({
  href,
  className,
  style,
  children,
}: {
  href: string
  className?: string
  style?: React.CSSProperties
  children: React.ReactNode
}) {
  return (
    <a
      href={href}
      onClick={(e) => {
        if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
        e.preventDefault()
        window.__cinqoToolBridge.navigate(href)
      }}
      className={className}
      style={style}
    >
      {children}
    </a>
  )
}
