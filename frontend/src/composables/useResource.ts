import { onBeforeUnmount, ref, shallowRef } from 'vue'
import { ApiError, isAbortError } from '@/api/campaigns'

/** One screen resource; an older request can never replace the current screen state. */
export function useResource<T>(fetchResource: (signal: AbortSignal) => Promise<T>) {
  const data = shallowRef<T | null>(null)
  const loading = ref(true)
  const error = ref('')
  let controller: AbortController | null = null
  async function reload() {
    controller?.abort()
    const current = new AbortController()
    controller = current
    loading.value = true
    error.value = ''
    try {
      const result = await fetchResource(current.signal)
      if (controller === current) data.value = result
    } catch (err) {
      if (controller === current && !isAbortError(err))
        error.value = err instanceof ApiError ? err.message : 'Veriler yüklenemedi.'
    } finally {
      if (controller === current) loading.value = false
    }
  }
  onBeforeUnmount(() => {
    controller?.abort()
    controller = null
  })
  return { data, loading, error, reload }
}
