import { useEffect, useRef, useState } from 'react';
import type { RuntimeExplorationCount } from '../../runtime/outputTypes.ts';

interface RatchetPeak {
  count: number;
  failed: number;
}

interface RatchetState {
  turnId: string | undefined;
  peaks: Record<string, RatchetPeak>;
}

// useRatchetCounts keeps per-kind exploration counters monotonically
// non-decreasing while a turn streams in, so the process-trace header never
// flickers a number backwards (e.g. 5 -> 3 -> 5 files) as runtime snapshots
// race each other. Counters reset whenever `turnId` changes. Implemented as
// state derived during render (the documented "adjusting state when a prop
// changes" pattern) rather than a ref, so it stays a pure render.
export function useRatchetCounts(
  counts: RuntimeExplorationCount[] | undefined,
  turnId: string | undefined,
): RuntimeExplorationCount[] | undefined {
  const [state, setState] = useState<RatchetState>({ turnId, peaks: {} });

  const basePeaks = state.turnId === turnId ? state.peaks : {};
  let nextPeaks = basePeaks;
  let peaksChanged = state.turnId !== turnId;

  if (counts) {
    for (const entry of counts) {
      const previous = nextPeaks[entry.kind];
      const nextCount = Math.max(previous?.count ?? 0, entry.count);
      const nextFailed = Math.max(previous?.failed ?? 0, entry.failed ?? 0);
      if (!previous || previous.count !== nextCount || previous.failed !== nextFailed) {
        if (nextPeaks === basePeaks) {
          nextPeaks = { ...basePeaks };
        }
        nextPeaks[entry.kind] = { count: nextCount, failed: nextFailed };
        peaksChanged = true;
      }
    }
  }

  if (peaksChanged) {
    setState({ turnId, peaks: nextPeaks });
  }

  if (!counts) {
    return counts;
  }

  return counts.map((entry) => {
    const ratcheted = nextPeaks[entry.kind];
    if (!ratcheted || (ratcheted.count === entry.count && ratcheted.failed === (entry.failed ?? 0))) {
      return entry;
    }
    return { kind: entry.kind, count: ratcheted.count, failed: ratcheted.failed || undefined };
  });
}

// useMinDisplay holds the previously rendered value for at least `minMs`
// milliseconds before adopting a newly received value. It smooths out
// high-frequency streaming updates (e.g. a status verb alternating between
// "正在探索" and "部分失败" as tool results race in) without ever dropping
// the final value. All wall-clock reads happen inside effects/timeouts, never
// during render.
export function useMinDisplay<T>(value: T, minMs = 700): T {
  const [displayed, setDisplayed] = useState(value);
  const lastSwitchAtRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (lastSwitchAtRef.current === undefined) {
      lastSwitchAtRef.current = Date.now();
    }
    if (Object.is(displayed, value)) {
      return undefined;
    }
    const elapsed = Date.now() - lastSwitchAtRef.current;
    const delay = Math.max(0, minMs - elapsed);
    const timer = window.setTimeout(() => {
      lastSwitchAtRef.current = Date.now();
      setDisplayed(value);
    }, delay);
    return () => window.clearTimeout(timer);
    // Only `value`/`minMs` should retrigger scheduling; `displayed` is the
    // hook's own derived state and re-running on it would fight the timer.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, minMs]);

  return displayed;
}

// useLatchedOpen backs a controlled collapse: it follows `autoOpen` until the
// user manually toggles it, at which point the manual choice wins until
// `resetKey` changes (e.g. the turn the trace belongs to moves to a new
// status generation).
export function useLatchedOpen(autoOpen: boolean, resetKey: string | undefined): [boolean, (value: boolean) => void] {
  const [state, setState] = useState<{ resetKey: string | undefined; manual?: boolean }>({ resetKey });

  if (state.resetKey !== resetKey) {
    setState({ resetKey });
  }

  const open = state.manual ?? autoOpen;
  const setOpen = (value: boolean) => setState((previous) => ({ ...previous, manual: value }));
  return [open, setOpen];
}
