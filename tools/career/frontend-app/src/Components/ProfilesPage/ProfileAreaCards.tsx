import { MenuGlyph } from '../../Shared/Icons/MenuGlyph'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { isPlainLeftClick, profileAreaLinks } from './profileAreas'

// The row at the bottom of a profile's card (plan 233): one tile per page of the profile (personas, summary, goals, skills, experience,
// education, languages, courses, hobbies), each with the menu's own icon. A tile is a real link: a plain click opens the page inside the tool,
// a middle or modified click leaves it to the browser. The page it opens shows this profile's data only.
export function ProfileAreaCards({ profileId, name }: { profileId: string; name: string }) {
  return (
    <nav aria-label={`Pages of ${name}`} className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
      {profileAreaLinks(profileId, name).map((a) => (
        <a
          key={a.key}
          href={a.href}
          title={a.title}
          onClick={(e) => {
            if (!isPlainLeftClick(e)) return
            e.preventDefault()
            window.__cinqoToolBridge.navigate(a.href)
          }}
          className="flex items-center gap-2 rounded-2xl border bg-white px-2.5 py-2 text-xs font-medium text-gray-700 shadow-sm transition-shadow hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500"
          style={{ borderColor: PAGE_ACCENT.soft }}
        >
          <span
            className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full"
            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
          >
            <MenuGlyph svg={a.icon} className="h-4 w-4" />
          </span>
          {a.label}
        </a>
      ))}
    </nav>
  )
}
