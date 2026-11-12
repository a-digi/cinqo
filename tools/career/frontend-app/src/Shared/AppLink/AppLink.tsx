// A link to another page of the app: a real <a>, so it can be opened in a new tab, whose plain click navigates
// inside the app instead of reloading it. A modified click (new tab, new window) keeps the link's own behaviour.
export function AppLink({
  href,
  className,
  style,
  'aria-label': ariaLabel,
  children,
}: {
  href: string
  className?: string
  style?: React.CSSProperties
  // A name for the link when its text alone does not say enough (several links read the same).
  'aria-label'?: string
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
      aria-label={ariaLabel}
    >
      {children}
    </a>
  )
}
