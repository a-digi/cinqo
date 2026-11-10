import { addLanguage, fetchLanguages, updateLanguage, type LanguageInput } from '../../api'
import { LanguagesIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { LANGUAGES_PATH } from '../../paths'
import { Field } from '../Field/Field'
import { ProfileEntryPage } from '../ProfileItems/ProfileEntryPage'
import { LEVELS, formProblem } from './languagesRules'

const emptyEntry: LanguageInput = { language: '', level: '', canSpeak: true, canWrite: false }

// The page behind "Add language" and each language's "Edit": ?profileId= says whose languages, ?id= which one to edit (none = add).
export function LanguagesEntryPage() {
  return (
    <ProfileEntryPage<LanguageInput>
      listPath={LANGUAGES_PATH}
      icon={<LanguagesIcon />}
      noun="language"
      backLabel="Back to languages"
      emptyForm={emptyEntry}
      load={async (profileId, entryId) => {
        const found = (await fetchLanguages(profileId)).find((l) => l.id === entryId)
        return found ? { language: found.language, level: found.level, canSpeak: found.canSpeak, canWrite: found.canWrite } : null
      }}
      problem={formProblem}
      save={(profileId, entryId, form) => (entryId ? updateLanguage(entryId, form) : addLanguage(profileId, form))}
      renderFields={(form, setForm) => (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label="Language"
              value={form.language}
              onChange={(v) => {
                setForm({ ...form, language: v })
              }}
            />
            <label className="block text-xs font-medium text-gray-500">
              Level
              <select
                value={form.level}
                onChange={(e) => {
                  setForm({ ...form, level: e.target.value })
                }}
                className={`mt-1 ${inputClass}`}
              >
                {LEVELS.map((l) => (
                  <option key={l.value} value={l.value}>
                    {l.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="mt-3 flex flex-wrap gap-5 text-sm text-gray-700">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form.canSpeak}
                onChange={(e) => {
                  setForm({ ...form, canSpeak: e.target.checked })
                }}
              />
              Spoken
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form.canWrite}
                onChange={(e) => {
                  setForm({ ...form, canWrite: e.target.checked })
                }}
              />
              Written
            </label>
          </div>
        </>
      )}
    />
  )
}
