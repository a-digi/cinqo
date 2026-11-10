import { fetchLanguages, removeLanguage, type Language } from '../../api'
import { LanguagesIcon } from '../../Shared/Icons/icons'
import { LANGUAGES_ENTRY_PATH, LANGUAGES_PATH } from '../../paths'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { ProfileItemsPage } from '../ProfileItems/ProfileItemsPage'
import { abilityText, languagesCountText, levelLabel } from './languagesRules'

// The Languages page (plan 231): the languages of the profile, spoken, written or both, with an optional level. Global to the profile;
// personas do not choose among them.
export function LanguagesPage() {
  return (
    <ProfileItemsPage<Language>
      path={LANGUAGES_PATH}
      entryPath={LANGUAGES_ENTRY_PATH}
      icon={<LanguagesIcon />}
      title="Languages"
      description="The languages you speak or write, with your level. They belong to the profile itself, so every target role and every CV can use them."
      tipsPage="languages"
      addLabel="Add language"
      hint="Each language is listed once; the best levels come first."
      emptyText="No languages yet. Add the first one with “Add language”."
      countText={languagesCountText}
      fetchItems={fetchLanguages}
      removeItem={removeLanguage}
      labelOf={(l) => l.language}
      renderItem={(l) => (
        <>
          <div className="text-sm font-semibold">{l.language}</div>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            {levelLabel(l.level) !== '' && (
              <span className="rounded-full px-2.5 py-0.5 text-xs font-medium" style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}>
                {levelLabel(l.level)}
              </span>
            )}
            <span className="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-600">{abilityText(l)}</span>
          </div>
        </>
      )}
    />
  )
}
