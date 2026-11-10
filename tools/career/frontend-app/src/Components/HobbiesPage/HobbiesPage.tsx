import { fetchHobbies, removeHobby, type Hobby } from '../../api'
import { HobbiesIcon } from '../../Shared/Icons/icons'
import { HOBBIES_ENTRY_PATH, HOBBIES_PATH } from '../../paths'
import { ProfileItemsPage } from '../ProfileItems/ProfileItemsPage'
import { hobbiesCountText } from './hobbiesRules'

// The Hobbies page (plan 231): what you do outside work. Global to the profile; personas do not choose among them.
export function HobbiesPage() {
  return (
    <ProfileItemsPage<Hobby>
      path={HOBBIES_PATH}
      entryPath={HOBBIES_ENTRY_PATH}
      icon={<HobbiesIcon />}
      title="Hobbies"
      description="What you do outside work. They belong to the profile itself, so every persona and every CV can use them."
      tipsPage="hobbies"
      addLabel="Add hobby"
      hint="Each hobby is listed once, by name."
      emptyText="No hobbies yet. Add the first one with “Add hobby”."
      countText={hobbiesCountText}
      fetchItems={fetchHobbies}
      removeItem={removeHobby}
      labelOf={(h) => h.name}
      renderItem={(h) => (
        <>
          <div className="text-sm font-semibold">{h.name}</div>
          {h.description !== '' && <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{h.description}</div>}
        </>
      )}
    />
  )
}
