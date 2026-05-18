import type { RuntimeEvent } from './types'

type RuntimeEventHandler = (event: RuntimeEvent) => void
type RuntimeEventErrorHandler = (error: Event) => void

export function subscribeRuntimeEvents(
  url: string,
  onEvent: RuntimeEventHandler,
  onError: RuntimeEventErrorHandler,
) {
  const source = new EventSource(url)

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

