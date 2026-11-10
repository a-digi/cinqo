import { addCourse, fetchCourses, updateCourse, type CourseInput } from '../../api'
import { CoursesIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { COURSES_PATH } from '../../paths'
import { Field } from '../Field/Field'
import { ProfileEntryPage } from '../ProfileItems/ProfileEntryPage'
import { formProblem } from './coursesRules'

const emptyEntry: CourseInput = { title: '', provider: '', completedDate: '', credentialUrl: '', description: '' }

// The page behind "Add course" and each course's "Edit": ?profileId= says whose courses, ?id= which one to edit (none = add).
export function CoursesEntryPage() {
  return (
    <ProfileEntryPage<CourseInput>
      listPath={COURSES_PATH}
      icon={<CoursesIcon />}
      noun="course"
      backLabel="Back to courses"
      emptyForm={emptyEntry}
      load={async (profileId, entryId) => {
        const c = (await fetchCourses(profileId)).find((x) => x.id === entryId)
        return c
          ? { title: c.title, provider: c.provider, completedDate: c.completedDate, credentialUrl: c.credentialUrl, description: c.description }
          : null
      }}
      problem={formProblem}
      save={(profileId, entryId, form) => (entryId ? updateCourse(entryId, form) : addCourse(profileId, form))}
      renderFields={(form, setForm) => (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label="Title"
              value={form.title}
              onChange={(v) => {
                setForm({ ...form, title: v })
              }}
            />
            <Field
              label="Provider"
              value={form.provider}
              onChange={(v) => {
                setForm({ ...form, provider: v })
              }}
            />
            <Field
              label="Completed (YYYY, YYYY-MM, empty = not given)"
              value={form.completedDate}
              onChange={(v) => {
                setForm({ ...form, completedDate: v })
              }}
            />
            <Field
              label="Certificate link (https://…)"
              value={form.credentialUrl}
              onChange={(v) => {
                setForm({ ...form, credentialUrl: v })
              }}
            />
          </div>
          <label className="mt-3 block text-xs font-medium text-gray-500">
            Description
            <textarea
              value={form.description}
              onChange={(e) => {
                setForm({ ...form, description: e.target.value })
              }}
              rows={4}
              className={`mt-1 ${inputClass}`}
            />
          </label>
        </>
      )}
    />
  )
}
