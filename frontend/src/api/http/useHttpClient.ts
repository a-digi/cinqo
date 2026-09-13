import { useContext } from 'react'
import { HttpClientContext, type HttpClientContextType } from './HttpClientContext'

export function useHttpClient(): HttpClientContextType {
  const ctx = useContext(HttpClientContext)
  if (!ctx) {
    throw new Error('useHttpClient must be used within an HttpClientProvider')
  }
  return ctx
}
