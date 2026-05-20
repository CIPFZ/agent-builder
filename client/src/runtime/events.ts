import type { RuntimeEvent } from './types'

type RuntimeEventHandler = (event: RuntimeEvent) => void
type RuntimeEventErrorHandler = (error: Event) => void

export function subscribeRuntimeEvents(
  url: string,
  token: string | undefined,
  onEvent: RuntimeEventHandler,
  onError: RuntimeEventErrorHandler,
) {
  const source = new EventSource(token ? `${url}${url.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}` : url)

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
