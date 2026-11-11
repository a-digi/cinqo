import { SectionList } from './SectionList'
import { COURSES, EDUCATION, HOBBIES, LANGUAGES } from './cvSectionRules'
import type { CvStepProps } from './PersonalDetailsStep'

// Wizard step 6 (plan 251): the education, courses, languages and hobbies of the profile, as they are on this CV. They start as the profile has them; the person
// takes out what this CV should not carry, corrects a line or adds one. Like every step it edits this CV's data only, never the profile.
export function MoreSectionsStep({ data, onChange }: CvStepProps) {
  return (
    <div className="space-y-3">
      <p className="text-xs text-gray-500">
        These come from the profile. Remove what you do not want on this CV, or change a line: it is only changed for this CV, never in the
        profile.
      </p>
      <SectionList
        spec={EDUCATION}
        items={data.education ?? []}
        onChange={(next) => {
          onChange({ ...data, education: next })
        }}
      />
      <SectionList
        spec={COURSES}
        items={data.courses ?? []}
        onChange={(next) => {
          onChange({ ...data, courses: next })
        }}
      />
      <SectionList
        spec={LANGUAGES}
        items={data.languages ?? []}
        onChange={(next) => {
          onChange({ ...data, languages: next })
        }}
      />
      <SectionList
        spec={HOBBIES}
        items={data.hobbies ?? []}
        onChange={(next) => {
          onChange({ ...data, hobbies: next })
        }}
      />
    </div>
  )
}
