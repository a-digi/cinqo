import { useEffect, useRef, useState } from 'react'
import {
  fetchPersonaDetails,
  fetchPersonas,
  fetchProfileImage,
  fetchProfiles,
  fetchPhotos,
  linkProfilePhoto,
  deletePhoto,
  uploadProfileImage,
  deleteProfileImage,
  mediaDownloadUrl,
  type Photo,
  type ProfileImage,
} from '../../api'
import { ProfileSwitcher } from '../ProfileSwitcher/ProfileSwitcher'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { CameraIcon, UploadIcon, XIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { EXPERIENCE_PATH, NEW_PERSONA_PATH, PERSONA_DETAILS_PATH, SKILLS_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { setPersonaIdInLocalStorage } from '../Persona/PersonaSwitcher/repositoryLocalStorage'
import { CvReadiness } from './CvReadiness'
import { readinessItems, type ReadinessItem } from './readinessRules'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PhotoLibrary } from './PhotoLibrary'
import { deletePhotoMessage } from './photoRules'

// A profile's own photo — always tied to a profile (never uploaded
// standalone), used from the CV Builder templates. Career's frontend is
// its own separate JS bundle from the core app's, so this reimplements
// a small uploader against Media's own HTTP API (via this tool's own
// /profile-image proxy route) rather than importing core's
// ImageUploadCropper.tsx directly — that component lives in a bundle
// this one cannot reach. See
// plan/ai/career/profile-image/step-01-overview.md. Header card, one tinted card
// with a round preview (the profile's initial until there is a photo) and, on its
// right, a CV-readiness checklist as the overview.
export function ProfileImagePage() {
  const [profileId, setProfileId] = useState<string | null>(null)
  const [image, setImage] = useState<ProfileImage | null>(null)
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  // A replace keeps the SAME media file id (see forwardReplaceImage,
  // profileimage/handler.go) — its download URL never changes, so the
  // browser's own HTTP cache would otherwise keep showing the old
  // bytes after a replace (verified live: a same-session <img> kept
  // its stale preview until a full page reload). Appending this as a
  // query param busts that cache without needing a new id.
  const [cacheBuster, setCacheBuster] = useState(0)
  // Unlinking is only this profile's link; deleting a photo is for good, from the library.
  const [confirmingUnlink, setConfirmingUnlink] = useState(false)
  const [photos, setPhotos] = useState<Photo[]>([])
  const [deletingPhoto, setDeletingPhoto] = useState<Photo | null>(null)
  const [linking, setLinking] = useState(false)
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
    const path = item.fix === 'details' ? PERSONA_DETAILS_PATH : item.fix === 'skills' ? SKILLS_PATH : EXPERIENCE_PATH
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
    fetchProfileImage(forProfileId)
      .then(setImage)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
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
      .then((updated) => {
        setImage(updated)
        setCacheBuster((n) => n + 1)
        loadPhotos()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setUploading(false)
        if (fileInputRef.current) fileInputRef.current.value = ''
      })
  }

  function handleUnlink() {
    if (!profileId) return
    setConfirmingUnlink(true)
  }

  function confirmUnlink() {
    if (!profileId) return
    setConfirmingUnlink(false)
    setError('')
    deleteProfileImage(profileId)
      .then(() => {
        setImage(null)
        loadPhotos()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handlePick(photo: Photo) {
    if (!profileId) return
    setError('')
    setLinking(true)
    linkProfilePhoto(profileId, photo.mediaFileId)
      .then((linked) => {
        setImage(linked)
        loadPhotos()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLinking(false)
      })
  }

  function confirmDeletePhoto() {
    const photo = deletingPhoto
    if (!photo) return
    setDeletingPhoto(null)
    setError('')
    deletePhoto(photo.mediaFileId)
      .then(() => {
        if (image?.mediaFileId === photo.mediaFileId) setImage(null)
        loadPhotos()
        if (profileId) loadReadiness(profileId)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="career-v-stretch career-a-float career-t-wave max-w-5xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<CameraIcon />}
        title="Photo"
        description="A profile's photo, used by the CV templates. A profile uses one photo at a time; one photo can be shared by many profiles."
        pill={profileId && !loading ? { text: image ? 'Photo set' : 'No photo' } : undefined}
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
                {image ? (
                  <img
                    src={`${mediaDownloadUrl(image.mediaFileId)}?v=${cacheBuster}`}
                    alt="Profile"
                    className="h-full w-full object-cover"
                  />
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
                  {image ? 'This photo is used on your CVs.' : 'No photo yet. Add one to use it on your CVs.'}
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
                  {uploading ? 'Uploading…' : image ? 'Replace photo' : 'Upload photo'}
                </button>
                {image && (
                  <button
                    type="button"
                    onClick={handleUnlink}
                    title="This profile stops using the photo. The photo stays in your library."
                    className="flex items-center gap-1.5 rounded-md border border-gray-200 bg-white px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
                  >
                    <XIcon />
                    Unlink photo
                  </button>
                )}
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
            <PhotoLibrary
              photos={photos}
              currentMediaFileId={image?.mediaFileId ?? ''}
              cacheBuster={cacheBuster}
              busy={linking || uploading}
              onPick={handlePick}
              onDelete={setDeletingPhoto}
            />
          </div>
        )}

        <aside className="w-full shrink-0 lg:ml-auto lg:sticky lg:top-6 lg:w-72">
          {profileId && (
            <>
              <CvReadiness
                items={readinessItems({ hasPhoto: image !== null, personaCount, checked })}
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
        open={confirmingUnlink}
        title="Unlink profile photo"
        message="This profile will stop using this photo. The photo stays in your library, and other profiles that use it keep it."
        confirmLabel="Unlink"
        onConfirm={confirmUnlink}
        onCancel={() => {
          setConfirmingUnlink(false)
        }}
      />
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
