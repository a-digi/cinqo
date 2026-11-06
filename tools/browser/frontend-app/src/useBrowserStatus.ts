import { useCallback, useEffect, useState } from 'react'
import {
  fetchBrowserSettings,
  fetchChromeStatus,
  fetchEngineStatus,
  saveBrowserSettings,
  testEngine,
  type BrowserSettings,
  type ChromeStatus,
  type EngineStatusResponse,
  type EngineTestResult,
  type FetchEngine,
  type TestableEngine,
} from './api'

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// What the "Browser & Engine" page knows about Chrome: is one installed on the computer
// where Cinqo runs (step 53). "Check again" re-reads it without restarting anything.
// wasMissing turns true once the page has seen Chrome missing, so that finding it later
// (after the user installed it) can offer the Reload: a tool's menu is registered once
// when the app loads, so the other Browser pages only show up after a reload.
export function useChromeStatus() {
  const [status, setStatus] = useState<ChromeStatus | null>(null)
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(true)
  const [wasMissing, setWasMissing] = useState(false)

  const check = useCallback(() => {
    setChecking(true)
    setError('')
    fetchChromeStatus()
      .then((next) => {
        setStatus(next)
        if (!next.installed) setWasMissing(true)
      })
      .catch((err: unknown) => {
        setError(message(err))
      })
      .finally(() => {
        setChecking(false)
      })
  }, [])

  useEffect(() => {
    check()
  }, [check])

  return { status, error, checking, wasMissing, check }
}

// The engine side of the page (step 50, availability step 51, test step 52): the
// settings (which engine, human assist), each engine's availability, and "Test
// Katana". The whole settings object is saved on every change (the backend
// overwrites it on a PUT), with the Debug values exactly as they were loaded.
export function useEngine() {
  const [settings, setSettings] = useState<BrowserSettings | null>(null)
  const [status, setStatus] = useState<EngineStatusResponse | null>(null)
  const [statusError, setStatusError] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // "Test Katana" really starts Chrome and attaches Katana, so a Chrome that cannot be driven
  // is caught. The result stays on screen until the next test.
  // Which engine's test is running (null: none), and the last result with the engine it was for.
  const [testing, setTesting] = useState<TestableEngine | null>(null)
  const [testResult, setTestResult] = useState<(EngineTestResult & { engine: TestableEngine }) | null>(null)
  const [testError, setTestError] = useState('')

  useEffect(() => {
    fetchBrowserSettings()
      .then(setSettings)
      .catch((err: unknown) => {
        setError(message(err))
      })
    // The status is advice, never a precondition: if it cannot be read the page still
    // works and the switch behaves as it did before availability existed.
    fetchEngineStatus()
      .then(setStatus)
      .catch((err: unknown) => {
        setStatusError(message(err))
      })
  }, [])

  function runTest(engineToTest: TestableEngine) {
    setTesting(engineToTest)
    setTestResult(null)
    setTestError('')
    testEngine(engineToTest)
      .then((r) => {
        setTestResult({ ...r, engine: engineToTest })
      })
      .catch((err: unknown) => {
        setTestError(message(err))
      })
      .finally(() => {
        setTesting(null)
      })
  }

  // Applies a change on top of the loaded settings and sends the whole object.
  function save(change: Partial<BrowserSettings>) {
    if (!settings) return
    const previous = settings
    setError('')
    setSaving(true)
    setSettings({ ...settings, ...change }) // optimistic — reverted below if the save fails
    saveBrowserSettings({ ...settings, ...change })
      .then(setSettings)
      .catch((err: unknown) => {
        setSettings(previous)
        setError(message(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  function chooseEngine(engine: FetchEngine) {
    save({ fetchEngine: engine })
  }

  return { settings, status, statusError, error, saving, testing, testResult, testError, runTest, save, chooseEngine }
}
