import { useEffect, useRef } from 'react'
import { useSnackBar } from '../Shared/Components/SnackBar/SnackBarContext'
import { DEDUPE_MS, DOWNLOAD_EVENT, downloadLinkOf, downloadMessage, downloadNameOf } from './downloadAnnounce'

// Says "Downloading ... Chrome saves it in your Downloads folder" in a snack bar whenever a download starts: a click on any link with a download
// attribute (the shell's, a tool's, a chat message's) or an announcement made with announceDownload. Renders nothing.
export function DownloadAnnouncer() {
  const { infoMessage } = useSnackBar()
  const last = useRef({ name: '', at: 0 })

  useEffect(() => {
    const say = (name: string) => {
      const now = Date.now()
      if (last.current.name === name && now - last.current.at < DEDUPE_MS) return
      last.current = { name, at: now }
      infoMessage(downloadMessage(name), 6000)
    }
    const onClick = (e: MouseEvent) => {
      const a = downloadLinkOf(e.target)
      if (a) say(downloadNameOf(a))
    }
    const onAnnounce = (e: Event) => {
      const detail: unknown = (e as CustomEvent).detail
      say(typeof detail === 'object' && detail !== null && 'name' in detail && typeof detail.name === 'string' ? detail.name : '')
    }
    document.addEventListener('click', onClick, true)
    window.addEventListener(DOWNLOAD_EVENT, onAnnounce)
    return () => {
      document.removeEventListener('click', onClick, true)
      window.removeEventListener(DOWNLOAD_EVENT, onAnnounce)
    }
  }, [infoMessage])

  return null
}
