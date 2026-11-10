// Draws one of the sidebar's own icons (menuIcons.ts: raw SVG strings written for the menu) at a size of the caller's choosing. The strings are
// first-party constants of this bundle, never user data, so setting them as markup is safe; the glyph is decoration (the caller's text says
// what it is).
export function MenuGlyph({ svg, className = 'h-5 w-5' }: { svg: string; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-flex shrink-0 [&>svg]:h-full [&>svg]:w-full ${className}`}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
