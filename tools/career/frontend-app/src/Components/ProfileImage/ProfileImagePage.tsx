import { useEffect, useRef, useState } from 'react'
import { fetchProfileImage, uploadProfileImage, deleteProfileImage, mediaDownloadUrl, type ProfileImage } from '../../api'
import { ProfileSwitcher } from '../ProfileSwitcher/ProfileSwitcher'
import { UploadIcon, TrashIcon } from '../../Shared/Icons/icons'

// A profile's own photo — always tied to a profile (never uploaded
// standalone), used from the CV Builder templates. Career's frontend is
// its own separate JS bundle from the core app's, so this reimplements
// a small uploader against Media's own HTTP API (via this tool's own
// /profile-image proxy route) rather than importing core's
// ImageUploadCropper.tsx directly — that component lives in a bundle
// this one cannot reach. See
// plan/ai/career/profile-image/step-01-overview.md.
export function ProfileImagePage() {
  const [profileId, setProfileId] = useState<string | null>(null)
  const [image, setImage] = useState<ProfileImage | null>(null)
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

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
    if (profileId) load(profileId)
  }, [profileId])

  function handleFileChange(file: File | null) {
    if (!file || !profileId) return
    setError('')
    setUploading(true)
    uploadProfileImage(profileId, file)
      .then(setImage)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setUploading(false)
        if (fileInputRef.current) fileInputRef.current.value = ''
      })
  }

  function handleDelete() {
    if (!profileId || !window.confirm('Remove this profile photo? This cannot be undone.')) return
    setError('')
    deleteProfileImage(profileId)
      .then(() => {
        setImage(null)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Image</h1>
      <p className="mb-5 text-sm text-gray-500">
        A profile's own photo, usable from the CV Builder's own templates. Each profile has at most one — uploading a new one replaces it.
      </p>

      <ProfileSwitcher profileId={profileId} onChange={setProfileId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {profileId && !loading && (
        <section className="flex items-start gap-5 rounded-lg border border-gray-200 p-4 shadow-sm">
          <div className="flex h-32 w-32 shrink-0 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-gray-50">
            {image ? (
              <img src={mediaDownloadUrl(image.mediaFileId)} alt="Profile" className="h-full w-full object-cover" />
            ) : (
              <span className="px-2 text-center text-xs text-gray-400">No photo yet</span>
            )}
          </div>

          <div className="flex flex-col gap-2">
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
                onClick={handleDelete}
                className="flex items-center gap-1.5 rounded-md border border-gray-200 px-3.5 py-2 text-sm text-red-700 hover:bg-red-50"
              >
                <TrashIcon />
                Remove photo
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
      )}
    </div>
  )
}
