import { useEffect, useRef } from 'react'
import type { RuntimeEvent } from './types'
import { subscribeRuntimeEvents } from './events'

type RuntimeEventSubscription = {
  enabled: boolean
  lastSequence?: number
  requestEndpoint: () => Promise<{ url: string; token?: string }>
  onEvent: (event: RuntimeEvent) => void
  onSnapshotRequired?: () => void
}

export function useRuntimeEventSubscription({
  enabled,
  lastSequence,
  requestEndpoint,
  onEvent,
  onSnapshotRequired,
}: RuntimeEventSubscription) {
  const lastSequenceRef = useRef(lastSequence)

  useEffect(() => {
    lastSequenceRef.current = lastSequence
  }, [lastSequence])

  useEffect(() => {
    if (!enabled) return
    let unsubscribe: (() => void) | undefined
    let cancelled = false

    requestEndpoint()
      .then(({ url, token }) => {
        if (cancelled || !url) return
        unsubscribe = subscribeRuntimeEvents(
          url,
          token,
          lastSequenceRef.current,
          (event) => {
            if (event.type === 'snapshot_required') {
              onSnapshotRequired?.()
              return
            }
            onEvent(event)
          },
          () => undefined,
        )
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      unsubscribe?.()
    }
  }, [enabled, onEvent, onSnapshotRequired, requestEndpoint])
}
