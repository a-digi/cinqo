import { addHobby, fetchHobbies, updateHobby, type HobbyInput } from '../../api'
import { HobbiesIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { HOBBIES_PATH } from '../../paths'
import { Field } from '../Field/Field'
import { ProfileEntryPage } from '../ProfileItems/ProfileEntryPage'
import { formProblem } from './hobbiesRules'

const emptyEntry: HobbyInput = { name: '', description: '' }

// The page behind "Add hobby" and each hobby's "Edit": ?profileId= says whose hobbies, ?id= which one to edit (none = add).
export function HobbiesEntryPage() {
  return (
    <ProfileEntryPage<HobbyInput>
      listPath={HOBBIES_PATH}
      icon={<HobbiesIcon />}
      noun="hobby"
      backLabel="Back to hobbies"
      emptyForm={emptyEntry}
      load={async (profileId, entryId) => {
        const h = (await fetchHobbies(profileId)).find((x) => x.id === entryId)
        return h ? { name: h.name, description: h.description } : null
      }}
      problem={formProblem}
      save={(profileId, entryId, form) => (entryId ? updateHobby(entryId, form) : addHobby(profileId, form))}
      renderFields={(form, setForm) => (
        <>
          <Field
            label="Name"
            value={form.name}
            onChange={(v) => {
              setForm({ ...form, name: v })
            }}
          />
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
