import { accents } from '../Shared/Components/CardArt/accents'

// One angular shape tucked behind the bottom of the sidebar: an outlined hexagon in the dashboard's violet accent at low opacity, cut by
// the bottom left corner, drifting slowly and softly all the time (cinqo-sidebar-hexagon in index.css; still for people who ask for less
// motion). Purely decorative: hidden from screen readers, never catches a click, and fixed to the panel (it does not scroll with the menu).
export function SidebarDecor() {
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden">
      <svg viewBox="0 0 100 100" className="cinqo-sidebar-hexagon absolute -bottom-10 -left-10 h-40 w-40">
        <polygon
          points="25,5 75,5 100,50 75,95 25,95 0,50"
          fill="none"
          stroke={accents.violet.to}
          strokeWidth={7}
          strokeLinejoin="round"
          opacity={0.14}
        />
      </svg>
    </div>
  )
}
