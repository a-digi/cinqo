import { createContext } from 'react'

export interface HttpClientContextType {
  get<T>(endpoint: string): Promise<T>
  post<T>(endpoint: string, body?: unknown): Promise<T>
  put<T>(endpoint: string, body?: unknown): Promise<T>
  patch<T>(endpoint: string, body?: unknown): Promise<T>
  del<T>(endpoint: string): Promise<T>
  // Multipart upload — no dedup (deduplicating on a FormData body isn't
  // meaningful, and uploads are the last thing to silently coalesce).
  upload<T>(endpoint: string, formData: FormData): Promise<T>
}

export const HttpClientContext = createContext<HttpClientContextType | null>(null)
