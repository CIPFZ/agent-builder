import type { RuntimeEvent } from './types'

type RuntimeEventHandler = (event: RuntimeEvent) => void
type RuntimeEventErrorHandler = (error: Event) => void

export function subscribeRuntimeEvents(
  url: string,
  token: string | undefined,
  after: number | undefined,
  onEvent: RuntimeEventHandler,
  onError: RuntimeEventErrorHandler,
) {
  const params = new URLSearchParams()
  if (token) params.set('token', token)
  if (after && after > 0) params.set('after', String(after))
  const separator = url.includes('?') ? '&' : '?'
  const source = new EventSource(params.size ? `${url}${separator}${params.toString()}` : url)

  source.addEventListener('runtime-event', (event) => {
    try {
      onEvent(JSON.parse(event.data) as RuntimeEvent)
    } catch {
      onError(event)
    }
  })
  source.onerror = onError

  return () => source.close()
}
