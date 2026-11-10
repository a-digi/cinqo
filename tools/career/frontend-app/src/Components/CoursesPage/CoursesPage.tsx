import { fetchCourses, removeCourse, type Course } from '../../api'
import { CoursesIcon } from '../../Shared/Icons/icons'
import { COURSES_ENTRY_PATH, COURSES_PATH } from '../../paths'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { ProfileItemsPage } from '../ProfileItems/ProfileItemsPage'
import { completedText, coursesCountText, isHttpLink } from './coursesRules'

// The Courses page (plan 231): the short trainings and certificates of the profile (schools and degrees are on the Education page).
// Global to the profile; personas do not choose among them.
export function CoursesPage() {
  return (
    <ProfileItemsPage<Course>
      path={COURSES_PATH}
      entryPath={COURSES_ENTRY_PATH}
      icon={<CoursesIcon />}
      title="Courses"
      description="The short trainings and certificates you completed. They belong to the profile itself, so every persona and every CV can use them."
      tipsPage="courses"
      addLabel="Add course"
      hint="Schools and degrees go on the Education page. The latest completed course comes first."
      emptyText="No courses yet. Add the first one with “Add course”."
      countText={coursesCountText}
      fetchItems={fetchCourses}
      removeItem={removeCourse}
      labelOf={(c) => c.title}
      renderItem={(c) => (
        <>
          <div className="text-sm font-semibold">{c.title}</div>
          {c.provider !== '' && <div className="text-xs text-gray-500">{c.provider}</div>}
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="rounded-full px-2.5 py-0.5 text-xs font-medium" style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}>
              {completedText(c.completedDate)}
            </span>
            {isHttpLink(c.credentialUrl) && (
              <a
                href={c.credentialUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700 hover:bg-blue-100"
              >
                Certificate
              </a>
            )}
          </div>
          {c.description !== '' && <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{c.description}</div>}
        </>
      )}
    />
  )
}
