import { useCallback, useEffect, useState } from 'react'
import { ApiError } from '../../../api/client'
import { AppScopes } from '../../../config/security/scopes'
import { useAuth } from '../../Auth/AuthContext'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { cancelDownload, type Download } from '../../../api/downloads'
import {
  fetchLocalAi,
  removeDownloads,
  startInstall,
  testModel,
  updateLocalAiSettings,
  type LocalAiResponse,
  type LocalAiTestResult,
} from './localAiApi'
import { hasDownloadedFiles, isDownloading } from './localAiFormat'
import { LocalAiForm } from './LocalAiForm'
import { draftOf, hasProblems, isDirty, problemsOf, toUpdate, type Draft } from './localAiRules'

function messageOf(err: unknown): string {
  return err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err)
}

// The Local AI settings page (AI → Local AI): switch the local model on, say where the server is and which model it
// serves. plan/ai/small-llm-local/step-181-local-ai-small-models.md
export function LocalAiPage() {
  const { scopes } = useAuth()
  const canManage = scopes.includes(AppScopes.SuperAdmin) || scopes.includes(AppScopes.PlatformManage)

  const [data, setData] = useState<LocalAiResponse | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [loadError, setLoadError] = useState('')
  const [saveError, setSaveError] = useState('')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)
  const [failures, setFailures] = useState<Partial<Record<string, Download>>>({})
  const [installBusy, setInstallBusy] = useState(false)
  const [installError, setInstallError] = useState('')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<LocalAiTestResult | null>(null)
  const [testError, setTestError] = useState('')
  const { confirm, dialog } = useConfirm()

  useEffect(() => {
    fetchLocalAi()
      .then((response) => {
        setData(response)
        setDraft(draftOf(response.settings))
      })
      .catch((err: unknown) => {
        setLoadError(messageOf(err))
      })
  }, [])

  // Reads the state of the files again (after a download ended, or was cancelled, or files were removed).
  const refresh = useCallback(() => {
    fetchLocalAi()
      .then((response) => {
        setData(response)
      })
      .catch((err: unknown) => {
        setInstallError(messageOf(err))
      })
  }, [])

  // While a test runs, the state of the model program is read now and then, so "starting" can be seen.
  useEffect(() => {
    if (!testing) return
    const timer = window.setInterval(refresh, 1500)
    return () => {
      window.clearInterval(timer)
    }
  }, [testing, refresh])

  function runTest() {
    if (!draft) return
    setTestError('')
    setTestResult(null)
    setTesting(true)
    testModel(draft.modelId === '' ? undefined : draft.modelId)
      .then(setTestResult)
      .catch((err: unknown) => {
        setTestError(messageOf(err))
      })
      .finally(() => {
        setTesting(false)
        refresh()
      })
  }

  function start(key: string, modelId?: string) {
    setInstallError('')
    setFailures((prev) => Object.fromEntries(Object.entries(prev).filter(([k]) => k !== key && k !== 'runtime')))
    setInstallBusy(true)
    startInstall(modelId)
      .then((response) => {
        setData(response)
      })
      .catch((err: unknown) => {
        setInstallError(messageOf(err))
      })
      .finally(() => {
        setInstallBusy(false)
      })
  }

  function cancel(downloadId: string) {
    setInstallError('')
    setInstallBusy(true)
    cancelDownload(downloadId)
      .catch((err: unknown) => {
        setInstallError(messageOf(err))
      })
      .finally(() => {
        setInstallBusy(false)
        refresh()
      })
  }

  const finished = useCallback(
    (key: string, d: Download | null) => {
      if (d?.state === 'failed') setFailures((prev) => ({ ...prev, [key]: d }))
      refresh()
    },
    [refresh],
  )

  function removeAll() {
    void confirm({
      title: 'Remove the downloaded files',
      message: 'This deletes the downloaded models and the program that runs them. You can download them again later.',
    }).then((ok) => {
      if (!ok) return
      setInstallError('')
      setInstallBusy(true)
      removeDownloads()
        .then(() => {
          setFailures({})
          refresh()
        })
        .catch((err: unknown) => {
          setInstallError(messageOf(err))
        })
        .finally(() => {
          setInstallBusy(false)
        })
    })
  }

  function save() {
    if (!data || !draft) return
    setSaveError('')
    setJustSaved(false)
    setSaving(true)
    // The page only shows the new values once the server has stored them: a refused change leaves everything as it was.
    updateLocalAiSettings(toUpdate(draft))
      .then((response) => {
        setData(response)
        setDraft(draftOf(response.settings))
        setJustSaved(true)
      })
      .catch((err: unknown) => {
        setSaveError(messageOf(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <div className="cinqo-stagger cinqo-v-swing cinqo-t-type max-w-3xl space-y-5 p-6 font-sans text-gray-900">
      <div>
        <h1 className="text-xl font-semibold">Local AI</h1>
        <p className="mt-1 text-sm text-gray-500">
          Let conversations be answered by a small language model that Cinqo runs on this computer instead of a cloud provider.
        </p>
      </div>
      {loadError ? (
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {loadError}
        </p>
      ) : !data || !draft ? (
        <div className="flex min-h-40 items-center justify-center">
          <LoadingSpinner label="Loading the local AI settings…" />
        </div>
      ) : (
        <LocalAiForm
          saved={data.settings}
          status={data.status}
          hardware={data.hardware}
          catalog={data.catalog}
          catalogCheckedOn={data.catalogCheckedOn}
          runtime={data.runtime}
          engine={data.engine}
          testing={testing}
          testResult={testResult}
          testError={testError}
          onTest={runTest}
          downloadedFiles={hasDownloadedFiles(data)}
          downloading={isDownloading(data)}
          failures={failures}
          installBusy={installBusy}
          installError={installError}
          onStartInstall={start}
          onCancelDownload={cancel}
          onDownloadFinished={finished}
          onRemoveDownloads={removeAll}
          draft={draft}
          problems={problemsOf(draft, data.catalog)}
          dirty={isDirty(data.settings, draft)}
          canManage={canManage}
          saving={saving}
          error={saveError}
          justSaved={justSaved}
          onChange={(next) => {
            setJustSaved(false)
            setDraft(next)
          }}
          onSave={() => {
            if (!hasProblems(problemsOf(draft, data.catalog))) save()
          }}
          onDiscard={() => {
            setSaveError('')
            setDraft(draftOf(data.settings))
          }}
        />
      )}
      {dialog}
    </div>
  )
}
