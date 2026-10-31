import { useEffect, useRef, useState } from 'react'
import {
  fetchPersonaDetails,
  fetchPersonas,
  fetchProfilePhotos,
  fetchProfiles,
  fetchPhotos,
  addProfilePhoto,
  makeDefaultPhoto,
  removeProfilePhoto,
  deletePhoto,
  uploadProfileImage,
  mediaDownloadUrl,
  type Photo,
  type ProfilePhoto,
} from '../../api'
import { ProfileSwitcher } from '../ProfileSwitcher/ProfileSwitcher'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { CameraIcon, UploadIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { EXPERIENCE_PATH, NEW_PERSONA_PATH, PERSONA_DETAILS_PATH, SKILLS_PATH, experiencePathFor, skillsPathFor } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { setPersonaIdInLocalStorage } from '../Persona/PersonaSwitcher/repositoryLocalStorage'
import { CvReadiness } from './CvReadiness'
import { readinessItems, type ReadinessItem } from './readinessRules'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PhotoLibrary } from './PhotoLibrary'
import { ProfilePhotos } from './ProfilePhotos'
import { defaultPhotoId, deletePhotoMessage, libraryToAdd } from './photoRules'

// A profile's own photos — always tied to a profile (never uploaded
// standalone), used from the CV Builder templates. A profile can have several, one of them its
// default (the one the templates use); a photo can be shared by many profiles and is kept in a
// library. Career's frontend is its own separate JS bundle from the core app's, so this
// reimplements a small uploader against Media's own HTTP API (via this tool's own /profile-image
// proxy route) rather than importing core's ImageUploadCropper.tsx directly — that component lives
// in a bundle this one cannot reach. See plan/ai/career/profile-image/step-01-overview.md and
// plan/ai/tools/career/step-111-many-photos-per-profile.md. Header card, one tinted card with a
// round preview of the default photo (the profile's initial until there is one), then the
// profile's photos and the library, and a CV-readiness checklist on the right as the overview.
export function ProfileImagePage() {
  const [profileId, setProfileId] = useState<string | null>(null)
  // The profile's own photos, the default first; the default is what the hero and the CV show.
  const [profilePhotos, setProfilePhotos] = useState<ProfilePhoto[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  // Every photo that exists (the library), for "Add from your library".
  const [photos, setPhotos] = useState<Photo[]>([])
  const [deletingPhoto, setDeletingPhoto] = useState<Photo | null>(null)
  const [working, setWorking] = useState(false)
  // The selected profile's name, for the placeholder shown until there is a photo.
  const [profileName, setProfileName] = useState('')
  // The CV-readiness check reads the profile's first persona (one extra read of two
  // existing routes); a failure only affects the checklist, never the photo card.
  const [persona, setPersona] = useState<{ id: string; name: string } | null>(null)
  const [personaCount, setPersonaCount] = useState(0)
  const [checked, setChecked] = useState<{ summary: string; skills: number; experience: number } | null>(null)
  const [readinessStatus, setReadinessStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const fileInputRef = useRef<HTMLInputElement>(null)
  // Only the newest readiness read may write to the screen (switching profile fast).
  const readinessSeq = useRef(0)

  function loadReadiness(forProfileId: string) {
    const seq = ++readinessSeq.current
    setReadinessStatus('loading')
    setPersona(null)
    setPersonaCount(0)
    setChecked(null)
    fetchPersonas(forProfileId)
      .then(async (personas) => {
        if (seq !== readinessSeq.current) return
        setPersonaCount(personas.length)
        const first = personas.at(0)
        if (first) {
          setPersona({ id: first.id, name: first.name })
          const details = await fetchPersonaDetails(first.id)
          if (seq !== readinessSeq.current) return
          setChecked({
            summary: details.personaDetails?.summary ?? '',
            skills: details.skills.length,
            experience: details.experience.length,
          })
        }
        setReadinessStatus('ready')
      })
      .catch(() => {
        if (seq === readinessSeq.current) setReadinessStatus('error')
      })
  }

  // "Fix": open the page that fixes the item, on the persona that was checked.
  function openFix(item: ReadinessItem) {
    if (!profileId) return
    if (item.fix === 'new-persona') {
      window.__cinqoToolBridge.navigate(`${NEW_PERSONA_PATH}?profileId=${encodeURIComponent(profileId)}`)
      return
    }
    if (persona) setPersonaIdInLocalStorage(persona.id)
    // A persona's skills have their own address; the other fixes still use the persona remembered above.
    const path =
      item.fix === 'details'
        ? PERSONA_DETAILS_PATH
        : item.fix === 'skills'
          ? persona
            ? skillsPathFor(persona.id)
            : SKILLS_PATH
          : persona
            ? experiencePathFor(persona.id)
            : EXPERIENCE_PATH
    window.__cinqoToolBridge.navigate(path)
  }

  // The library is shared by every profile; a failed read only hides the picker.
  function loadPhotos() {
    fetchPhotos()
      .then(setPhotos)
      .catch(() => {
        setPhotos([])
      })
  }

  function load(forProfileId: string) {
    setError('')
    setLoading(true)
    fetchProfilePhotos(forProfileId)
      .then(setProfilePhotos)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }

  // After a change: reread this profile's photos and the library, without the page going blank.
  function refresh(forProfileId: string) {
    fetchProfilePhotos(forProfileId)
      .then(setProfilePhotos)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    loadPhotos()
  }

  useEffect(() => {
    if (!profileId) return
    load(profileId)
    fetchProfiles()
      .then((list) => {
        const found = list.find((p) => p.id === profileId)
        setProfileName(found ? profileLabel(found) : '')
      })
      .catch(() => {
        setProfileName('')
      })
    loadReadiness(profileId)
    loadPhotos()
  }, [profileId])

  function handleFileChange(file: File | null) {
    if (!file || !profileId) return
    setError('')
    setUploading(true)
    uploadProfileImage(profileId, file)
      .then(() => {
        refresh(profileId)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setUploading(false)
        if (fileInputRef.current) fileInputRef.current.value = ''
      })
  }

  // Runs one change to this profile's photos, then rereads them.
  function change(action: (forProfileId: string) => Promise<unknown>) {
    if (!profileId) return
    setError('')
    setWorking(true)
    action(profileId)
      .then(() => {
        refresh(profileId)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setWorking(false)
      })
  }

  function confirmDeletePhoto() {
    const photo = deletingPhoto
    if (!photo) return
    setDeletingPhoto(null)
    change(() => deletePhoto(photo.mediaFileId))
  }

  const defaultId = defaultPhotoId(profilePhotos)

  return (
    <div className="career-v-stretch career-a-float career-t-wave max-w-5xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<CameraIcon />}
        title="Photo"
        description="A profile's photos, used by the CV templates. A profile can have several, one of them the default; one photo can be shared by many profiles."
        pill={
          profileId && !loading
            ? { text: profilePhotos.length === 0 ? 'No photo' : profilePhotos.length === 1 ? '1 photo' : `${profilePhotos.length} photos` }
            : undefined
        }
      />
      <ProfileSwitcher profileId={profileId} onChange={setProfileId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        {profileId && !loading && (
          <div className="flex min-w-0 flex-1 flex-col gap-5">
            <section
              className="career-enter career-delay-1 flex min-w-0 flex-col items-center gap-6 rounded-3xl border p-6 shadow-sm sm:flex-row sm:items-center"
              style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
            >
              <div className="flex h-36 w-36 shrink-0 items-center justify-center overflow-hidden rounded-full border-4 border-white shadow-md">
                {defaultId ? (
                  <img src={mediaDownloadUrl(defaultId)} alt="Profile" className="h-full w-full object-cover" />
                ) : (
                  <span
                    className="flex h-full w-full items-center justify-center text-5xl font-bold text-white"
                    style={{ background: accentFor(profileName).to }}
                    aria-label="No photo yet"
                  >
                    {initialOf(profileName)}
                  </span>
                )}
              </div>

              <div className="flex flex-col gap-2">
                <div className="text-sm text-gray-600">
                  {defaultId ? 'This is the default photo, the one your CVs use.' : 'No photo yet. Add one to use it on your CVs.'}
                </div>
                <button
                  type="button"
                  onClick={() => {
                    fileInputRef.current?.click()
                  }}
                  disabled={uploading}
                  className="flex items-center gap-1.5 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
                >
                  <UploadIcon className="h-3.5 w-3.5" />
                  {uploading ? 'Uploading…' : defaultId ? 'Add another photo' : 'Upload photo'}
                </button>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/*"
                  onChange={(e) => {
                    handleFileChange(e.target.files?.[0] ?? null)
                  }}
                  className="hidden"
                />
              </div>
            </section>
            <ProfilePhotos
              photos={profilePhotos}
              busy={working || uploading}
              onMakeDefault={(photo) => {
                change((id) => makeDefaultPhoto(id, photo.mediaFileId))
              }}
              onRemove={(photo) => {
                change((id) => removeProfilePhoto(id, photo.mediaFileId))
              }}
            />
            <PhotoLibrary
              photos={libraryToAdd(photos, profilePhotos)}
              busy={working || uploading}
              onAdd={(photo) => {
                change((id) => addProfilePhoto(id, photo.mediaFileId))
              }}
              onDelete={setDeletingPhoto}
            />
          </div>
        )}

        <aside className="w-full shrink-0 lg:ml-auto lg:sticky lg:top-6 lg:w-72">
          {profileId && (
            <>
              <CvReadiness
                items={readinessItems({ hasPhoto: defaultId !== null, personaCount, checked })}
                status={readinessStatus}
                personaName={persona?.name ?? ''}
                onFix={openFix}
              />
            </>
          )}
          <div className={profileId ? 'mt-8' : ''}>
            <PageTips page="photo" orientation="vertical" />
          </div>
        </aside>
      </div>

      <ConfirmationModal
        open={deletingPhoto !== null}
        title="Delete photo"
        message={deletePhotoMessage(deletingPhoto?.usedBy ?? [])}
        confirmLabel="Delete for good"
        variant="danger"
        onConfirm={confirmDeletePhoto}
        onCancel={() => {
          setDeletingPhoto(null)
        }}
      />
    </div>
  )
}
