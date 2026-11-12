import { useState } from 'react'
import {
  uploadCV,
  fetchCvText,
  storeDraftPiece,
  finishDraftPieces,
  createCVImportRun,
  finishCVImportRun,
  type CVUploadResult,
} from '../../api'
import { createConversation, sendMessage } from '../../Cinqo/Conversation/conversation'
import { fetchPlatforms } from '../../Cinqo/Platform/platformRepository'
import { needsShortStepsFor, usesCodeDriver } from '../../Cinqo/Platform/modelLabels'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { resolveImportAi, type ImportAiOverride } from './importAiChoice'
import {
  IMPORT_RUN_TOOLS,
  IMPORT_STEP_EXPECTATIONS,
  IMPORT_STEP_TOOLS,
  buildDoneNudge,
  buildImportPrompt,
  buildImportSteps,
  buildStepCorrection,
  saysStepDone,
} from './buildImportPrompt'
import { RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from '../../PersonaAi/personaTools'
import { analyseCv, analyseCvByCode } from './analysisFlow'
import { prepareCodeImport } from './codeImport/runCodeImport'
import { importContextProblem } from './importContext'
import { importAiState, NEEDS_AI_REASON } from './importAiRules'
import { importCvRunPathFor } from '../../paths'

// Where an import start is: the upload form, the analysis under way, or a failure.
export type ImportStartState = 'upload' | 'analyzing' | 'error'

// Everything that starts a CV import, taken out of ImportCvPage (plan/ai/tools/career/step-242-restart-cv-import-inside-the-import-page.md, step 1) so
// that the import's own page can start one as well: the AI choice for this import (plan 212), the upload, and the analysis (the code driver, the
// step-by-step way or the one long prompt) up to the moment the run is recorded and the page moves to the run's own address. No behaviour of the
// upload page changed by moving it. onRunsChanged is called when the history changed (a run was recorded or ended). targetProfileId (plan 261) is the
// profile the import goes into ('' = a new profile made from the CV): it is recorded on the run when the run is made.
export function useImportStart({ onRunsChanged, targetProfileId = '' }: { onRunsChanged: () => void; targetProfileId?: string }) {
  const [state, setState] = useState<ImportStartState>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [fileName, setFileName] = useState('')
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const [uploadResult, setUploadResult] = useState<CVUploadResult | null>(null)
  const [conversationId, setConversationId] = useState<string | null>(null)
  // The AI's last message (one plain sentence since step 155: the proposal itself arrives through the suggest_cv_import tool) —
  // kept so the 'error' state can still show it if the analysis fails.
  const [rawReply, setRawReply] = useState('')
  // The AI platform and model start from the one saved setting for the whole tool (Career ▸ Settings, or the automatic pick); the person may pick
  // another platform and model for this import (plan 212). That pick is for this page only: it is not saved and not remembered, so opening the page
  // starts from the Career settings again. handleUpload surfaces "no platform" as a real error if there is none.
  const ai = useAiChoice()
  const [aiOverride, setAiOverride] = useState<ImportAiOverride | null>(null)
  const settingsAi = { platform: ai.platform, model: ai.model }
  const chosen = resolveImportAi(settingsAi, ai.platforms, aiOverride)
  const platform = chosen.platform
  const platformsLoaded = ai.loaded
  // Whether an AI is set up at all (remote with a key, or local): without one nothing can be imported and the page says how to set one up.
  const aiState = importAiState(ai.loaded, ai.platforms.length)

  function pickFile(next: File | null) {
    setError('')
    setFile(next)
  }

  // runAnalysis is the shared tail of both a fresh upload
  // (handleUpload) and reprocessing an existing one
  // (handleProcessAgain) — everything from "we have a fileId" onward:
  // create the AI conversation, record the run, send the prompt, and
  // when the AI's turn ends read the proposal it handed over and enter
  // the review state. A reprocess of an upload whose underlying Media
  // file has since expired surfaces as a normal 'error' state here
  // (pdf_to_markdown's own tool-unavailable text) — no separate "is this
  // still live" check is made beforehand.
  //
  // Step 155: the AI does not print the proposal (the chat would show a JSON object). The run is recorded first ("generating");
  // the AI hands the proposal over by calling suggest_cv_import with the run id, which stores it; its last message is one plain
  // sentence. Only this page can end the run (the AI's turn lives in the core app), so it does, as ready or failed. See
  // plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
  async function runAnalysis(fileId: string, originalFilename: string, target: string) {
    setFileName(originalFilename)
    setState('analyzing')
    try {
      if (!platform) {
        throw new Error('No AI platform is configured — ask an admin to add one under Platforms.')
      }
      const model = chosen.model ?? undefined
      // A model with the code driver (Phi-4-mini, plan 209): the code reads and cuts the CV and asks the model small questions with no tools.
      // A CV the code cannot cut (no heading it knows, no text) goes the step-by-step way below instead.
      if (usesCodeDriver(platform, model)) {
        const prep = await prepareCodeImport(() => fetchCvText(fileId))
        if (prep.ok) {
          await analyseCvByCode(
            {
              startConversation: async () =>
                (await createConversation({ title: `Import CV: ${originalFilename}`, platformId: platform.id, model })).id,
              recordRun: (conversationId) =>
                createCVImportRun({ fileId, originalFilename, conversationId, driver: 'code', targetProfileId: target }),
              // Every question is a fresh turn of the run's own conversation (plan 210): the chat shows each question and its answer as they happen,
              // and the model is sent that question only (a small model repeats what it sees of its earlier answers). None of them is offered a tool.
              ask: async (prompt, conversationId) => (await sendMessage(conversationId, prompt, { fresh: true })).content,
              store: storeDraftPiece,
              finish: finishDraftPieces,
              endRun: finishCVImportRun,
              onConversation: setConversationId,
              onRun: (runId) => {
                window.__cinqoToolBridge.navigate(importCvRunPathFor(runId))
              },
              onRunsChanged,
            },
            prep.cv,
          )
          return
        }
      }
      const stepByStep = needsShortStepsFor(platform)
      await analyseCv({
        startConversation: async () => {
          // The AI may only use the tools an import needs: the server enforces it, not just the prompt. Unrestricted, a weak model
          // called create_profile with an invented name and wrote it into the user's data (plan 165).
          // Step by step (plan 210) every step then narrows this list to its own tools, per message.
          const conversation = await createConversation({
            title: `Import CV: ${originalFilename}`,
            platformId: platform.id,
            model,
            allowedTools: [...IMPORT_RUN_TOOLS],
          })
          if (!restrictionHolds(conversation.allowedTools, IMPORT_RUN_TOOLS)) throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
          return conversation.id
        },
        recordRun: (conversationId) => createCVImportRun({ fileId, originalFilename, conversationId, targetProfileId: target }),
        askAi: async (conversationId, runId, followUp, settings) => {
          const message = await sendMessage(conversationId, followUp ?? buildImportPrompt(fileId, runId), settings)
          return { reply: message.content, toolErrors: message.toolErrors, toolsCalled: message.toolsCalled }
        },
        // A platform whose models do better with short one-action messages (the local models: needsShortSteps) gets the instruction
        // in short steps instead of the one long prompt, which a small model answers with a made-up failure instead of a tool call.
        stepMessages: stepByStep ? (runId) => buildImportSteps(fileId, runId) : undefined,
        // Step-by-step only (plan 210): every step, and a continuation, is a fresh turn of the run's own conversation offered only the tools of that step
        // (every tool of the import for a continuation), so the model has none of its earlier replies to copy and the chat shows each step; and a step that
        // did not call the tools it had to is asked once more, then stops the run with a message that names the step.
        stepTools: stepByStep
          ? (index) => (index === undefined ? [...IMPORT_RUN_TOOLS] : [...(IMPORT_STEP_TOOLS[index] ?? IMPORT_RUN_TOOLS)])
          : undefined,
        saidDone: stepByStep ? saysStepDone : undefined,
        doneNudge: stepByStep ? buildDoneNudge : undefined,
        stepCorrection: stepByStep
          ? (index, verdict, runId) => buildStepCorrection(IMPORT_STEP_EXPECTATIONS[index], verdict, fileId, runId)
          : undefined,
        endRun: finishCVImportRun,
        onConversation: setConversationId,
        // The run exists: its own page takes over from here (the analysis goes on and ends the run; the page polls it).
        onRun: (runId) => {
          window.__cinqoToolBridge.navigate(importCvRunPathFor(runId))
        },
        onReply: setRawReply,
        onRunsChanged,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setState('error')
    }
  }

  // Whether the chosen platform can hold an import, read fresh (an admin may have just raised the context): the message to show, or
  // null. A failed read never blocks the import; the server's own refusal stays the safety net.
  async function contextProblem(): Promise<string | null> {
    if (!platform) return null
    try {
      const live = (await fetchPlatforms()).find((p) => p.id === platform.id)
      return importContextProblem(live?.contextTokens)
    } catch {
      return null
    }
  }

  async function handleUpload() {
    if (!file || aiState === 'needs-ai') return
    setUploading(true)
    setError('')

    // Before the upload: a refusal here keeps the chosen file, nothing is consumed.
    const tooSmall = await contextProblem()
    if (tooSmall) {
      setError(tooSmall)
      setUploading(false)
      return
    }

    let upload: CVUploadResult
    try {
      upload = await uploadCV(file)
      setUploadResult(upload)
      setUploading(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setUploading(false)
      return // stays on the 'upload' state — the inline error above already shows it
    }

    // The upload itself succeeded — any failure from here on is a real
    // 'error' state, not an inline retry-in-place message, since the
    // uploaded file (and its capability URL) are already consumed.
    await runAnalysis(upload.fileId, file.name, targetProfileId)
  }

  // Starts an analysis of a CV that is already stored (an earlier upload), as "Process again" does. Returns the reason it did not start (a platform
  // whose context is too small), or null when it started. The profile is the hook's own unless `target` is given (a "Process again" of an old run keeps
  // that run's own: '' means that run had none).
  async function startFromStored(fileId: string, originalFilename: string, target?: string): Promise<string | null> {
    if (aiState === 'needs-ai') return NEEDS_AI_REASON
    const tooSmall = await contextProblem()
    if (tooSmall) return tooSmall
    await runAnalysis(fileId, originalFilename, target ?? targetProfileId)
    return null
  }

  function startOver() {
    setState('upload')
    setFile(null)
    setFileName('')
    setUploadResult(null)
    setConversationId(null)
    setRawReply('')
    setError('')
  }

  function viewTranscript() {
    if (conversationId) window.__cinqoToolBridge.openConversation(conversationId)
  }

  return {
    state,
    file,
    fileName,
    uploading,
    error,
    uploadResult,
    conversationId,
    rawReply,
    chosen,
    settingsAi,
    platforms: ai.platforms,
    platformsLoaded,
    aiState,
    // Reads the platforms and keys again (after the person set an AI up on another page).
    recheckAi: ai.reload,
    changeAi: (platformId: string, model: string) => {
      setAiOverride({ platformId, model })
    },
    resetAi: () => {
      setAiOverride(null)
    },
    pickFile,
    upload: handleUpload,
    startFromStored,
    startOver,
    viewTranscript,
  }
}

export type ImportStart = ReturnType<typeof useImportStart>
