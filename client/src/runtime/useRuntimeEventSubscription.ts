import { useEffect } from 'react'
import type { RuntimeEvent } from './types'
import { subscribeRuntimeEvents } from './events'

type RuntimeEventSubscription = {
  enabled: boolean
  requestEndpoint: () => Promise<{ url: string; token?: string }>
  onEvent: (event: RuntimeEvent) => void
}

export function useRuntimeEventSubscription({ enabled, requestEndpoint, onEvent }: RuntimeEventSubscription) {
  useEffect(() => {
    if (!enabled) return
    let unsubscribe: (() => void) | undefined
    let cancelled = false

    requestEndpoint()
      .then(({ url, token }) => {
        if (cancelled || !url) return
        unsubscribe = subscribeRuntimeEvents(url, token, onEvent, () => undefined)
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      unsubscribe?.()
    }
  }, [enabled, onEvent, requestEndpoint])
}
