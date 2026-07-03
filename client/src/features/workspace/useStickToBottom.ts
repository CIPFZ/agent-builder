import { useCallback, useEffect, useRef, useState } from 'react';
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react';

/**
 * Sticky-bottom scroll state machine for the conversation timeline.
 *
 * `pinned` decides whether the container should keep following content
 * growth (streaming text, new timeline items, expanding tool cards, image
 * loads) by forcing `scrollTop = scrollHeight` on every relevant change.
 *
 * The state is changed ONLY by explicit signals, never by passively reading
 * scroll position after a scroll we caused ourselves:
 *
 *   UNPIN (pinned -> false), each detected from a real user gesture, never
 *   from a bare `scroll` event's distance-to-bottom:
 *     - wheel with deltaY < 0 (scrolling content up)
 *     - touch drag where the finger moves down the screen (content moves
 *       toward the top)
 *     - keydown PageUp / Home / ArrowUp, while focus is not inside an
 *       editable control (so composer text editing isn't mistaken for
 *       scroll navigation)
 *     - dragging the scrollbar thumb (pointerdown near the scrollbar track,
 *       followed by scroll events that move away from the bottom)
 *
 *   PIN (pinned -> true):
 *     - clicking the "jump to bottom" button
 *     - a submitted prompt (caller invokes `pinAndScrollToBottom`)
 *     - an active-session change (caller invokes `pinAndScrollToBottom`)
 *     - any genuine (non-programmatic) scroll event that lands within
 *       STICK_THRESHOLD_PX of the bottom - this direction is safe to infer
 *       from raw distance because it only ever re-engages the follow
 *       behavior, it can never cause the "stuck mid-scroll" bug pinning
 *       tries to avoid
 *
 * PROGRAMMATIC SCROLL IMMUNITY: every scrollTop mutation performed by this
 * hook sets `programmaticScrollRef.current = true` first. The scroll
 * handler ignores distance-based pin/unpin logic while that flag is set
 * (it still refreshes jump-button visibility). The flag is cleared by the
 * browser's `scrollend` event where supported, and unconditionally by a
 * 150ms debounce timer as a fallback for engines that don't fire
 * `scrollend` (WebView2).
 *
 * FOLLOW EXECUTION: while pinned, a ResizeObserver watches every direct
 * child of the scroll container (timeline column, todo bar, composer/
 * permission dock, ...) and re-asserts scrollTop = scrollHeight instantly
 * (never `smooth` - a smooth animation can't keep up with high-frequency
 * streaming growth and fights itself) whenever any of them changes size.
 * Children are (re)enumerated on every render so newly mounted nodes (e.g.
 * the permission dock swapping in for the composer) get observed too.
 *
 * A direct-child ResizeObserver was used instead of wrapping the
 * conversation content in an extra wrapper div: `Workspace.module.css`
 * relies on `.chatContent > [data-testid='composer']` (a direct-child
 * selector) for the sticky composer, and the jump-to-bottom button is
 * absolutely positioned against the nearest positioned ancestor. Wrapping
 * a subset of children risks breaking either. Observing the existing
 * top-level children avoids that risk entirely while still catching any
 * content growth, since a descendant growing always resizes the top-level
 * child that contains it.
 */

const STICK_THRESHOLD_PX = 48;
const JUMP_BUTTON_THRESHOLD_PX = 180;
const PROGRAMMATIC_SCROLL_DEBOUNCE_MS = 150;
const SCROLLBAR_HIT_ZONE_PX = 16;

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;
}

export function useStickToBottom() {
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const pinnedRef = useRef(true);
  const [pinned, setPinned] = useState(true);
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);

  const programmaticScrollRef = useRef(false);
  const programmaticScrollTimerRef = useRef<number | undefined>(undefined);
  const scrollbarDragActiveRef = useRef(false);
  const touchStartYRef = useRef<number | null>(null);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);
  const observedChildrenRef = useRef<Set<Element>>(new Set());
  const detachRef = useRef<() => void>(() => {});

  const setPinnedState = useCallback((next: boolean) => {
    if (pinnedRef.current === next) {
      return;
    }
    pinnedRef.current = next;
    setPinned(next);
  }, []);

  const distanceToBottom = useCallback(() => {
    const node = nodeRef.current;
    if (!node) {
      return Infinity;
    }
    return node.scrollHeight - node.scrollTop - node.clientHeight;
  }, []);

  const updateJumpVisibility = useCallback(() => {
    setShowJumpToBottom(distanceToBottom() > JUMP_BUTTON_THRESHOLD_PX);
  }, [distanceToBottom]);

  const endProgrammaticScroll = useCallback(() => {
    programmaticScrollRef.current = false;
    if (programmaticScrollTimerRef.current !== undefined) {
      window.clearTimeout(programmaticScrollTimerRef.current);
      programmaticScrollTimerRef.current = undefined;
    }
  }, []);

  const beginProgrammaticScroll = useCallback(() => {
    programmaticScrollRef.current = true;
    if (programmaticScrollTimerRef.current !== undefined) {
      window.clearTimeout(programmaticScrollTimerRef.current);
    }
    programmaticScrollTimerRef.current = window.setTimeout(() => {
      programmaticScrollRef.current = false;
      programmaticScrollTimerRef.current = undefined;
    }, PROGRAMMATIC_SCROLL_DEBOUNCE_MS);
  }, []);

  const scrollToBottomNow = useCallback(
    (behavior: ScrollBehavior) => {
      const node = nodeRef.current;
      if (!node) {
        return;
      }
      beginProgrammaticScroll();
      if (behavior === 'smooth') {
        node.scrollTo({ top: node.scrollHeight, behavior: 'smooth' });
      } else {
        node.scrollTop = node.scrollHeight;
      }
    },
    [beginProgrammaticScroll],
  );

  const jumpToBottom = useCallback(() => {
    setPinnedState(true);
    scrollToBottomNow('smooth');
    setShowJumpToBottom(false);
  }, [scrollToBottomNow, setPinnedState]);

  const pinAndScrollToBottom = useCallback(
    (behavior: ScrollBehavior = 'auto') => {
      setPinnedState(true);
      scrollToBottomNow(behavior);
      setShowJumpToBottom(false);
    },
    [scrollToBottomNow, setPinnedState],
  );

  const handleScroll = useCallback(
    () => {
      const node = nodeRef.current;
      if (!node) {
        return;
      }
      if (programmaticScrollRef.current && !scrollbarDragActiveRef.current) {
        return;
      }
      const distance = distanceToBottom();
      if (scrollbarDragActiveRef.current) {
        // Direct scrollbar-thumb drag: distance is a faithful, real-time
        // proxy for user intent in both directions.
        setPinnedState(distance <= STICK_THRESHOLD_PX);
      } else if (distance <= STICK_THRESHOLD_PX) {
        // Any other genuine (non-programmatic) scroll landing near the
        // bottom re-engages follow mode. This branch only ever pins, so it
        // can't reproduce the "self-unpinning smooth scroll" bug.
        setPinnedState(true);
      }
      setShowJumpToBottom(distance > JUMP_BUTTON_THRESHOLD_PX);
    },
    [distanceToBottom, setPinnedState],
  );

  const handleKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (isEditableTarget(event.target)) {
        return;
      }
      if (event.key === 'PageUp' || event.key === 'Home' || event.key === 'ArrowUp') {
        setPinnedState(false);
      }
    },
    [setPinnedState],
  );

  const handlePointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const node = nodeRef.current;
    if (!node) {
      return;
    }
    const rect = node.getBoundingClientRect();
    const nearScrollbar = event.clientX >= rect.right - SCROLLBAR_HIT_ZONE_PX;
    if (!nearScrollbar) {
      return;
    }
    scrollbarDragActiveRef.current = true;
    const clearDrag = () => {
      scrollbarDragActiveRef.current = false;
      window.removeEventListener('pointerup', clearDrag);
      window.removeEventListener('pointercancel', clearDrag);
    };
    window.addEventListener('pointerup', clearDrag);
    window.addEventListener('pointercancel', clearDrag);
  }, []);

  // Wheel and touchmove need a *passive native* listener (not React's JSX
  // handlers) so the browser never suspects them of blocking scroll.
  const containerRef = useCallback(
    (node: HTMLDivElement | null) => {
      detachRef.current();
      detachRef.current = () => {};
      nodeRef.current = node;
      if (!node) {
        resizeObserverRef.current?.disconnect();
        resizeObserverRef.current = null;
        observedChildrenRef.current.clear();
        return;
      }

      // Fresh container surface: default to following.
      pinnedRef.current = true;
      setPinned(true);
      setShowJumpToBottom(false);

      const onWheel = (event: WheelEvent) => {
        if (event.deltaY < 0) {
          setPinnedState(false);
        }
      };
      const onTouchStart = (event: TouchEvent) => {
        touchStartYRef.current = event.touches[0]?.clientY ?? null;
      };
      const onTouchMove = (event: TouchEvent) => {
        const startY = touchStartYRef.current;
        const currentY = event.touches[0]?.clientY;
        if (startY == null || currentY == null) {
          return;
        }
        if (currentY - startY > 0) {
          // Finger moving down the screen drags earlier content into view.
          setPinnedState(false);
        }
        touchStartYRef.current = currentY;
      };
      const onScrollEnd = () => {
        endProgrammaticScroll();
      };

      node.addEventListener('wheel', onWheel, { passive: true });
      node.addEventListener('touchstart', onTouchStart, { passive: true });
      node.addEventListener('touchmove', onTouchMove, { passive: true });
      node.addEventListener('scrollend', onScrollEnd);

      if (typeof ResizeObserver !== 'undefined') {
        resizeObserverRef.current = new ResizeObserver(() => {
          if (!pinnedRef.current) {
            updateJumpVisibility();
            return;
          }
          scrollToBottomNow('auto');
        });
      }

      detachRef.current = () => {
        node.removeEventListener('wheel', onWheel);
        node.removeEventListener('touchstart', onTouchStart);
        node.removeEventListener('touchmove', onTouchMove);
        node.removeEventListener('scrollend', onScrollEnd);
        resizeObserverRef.current?.disconnect();
        resizeObserverRef.current = null;
        observedChildrenRef.current.clear();
        endProgrammaticScroll();
      };
    },
    [endProgrammaticScroll, scrollToBottomNow, setPinnedState, updateJumpVisibility],
  );

  // Re-sync the ResizeObserver's watch list against the container's current
  // direct children on every render (cheap: a handful of top-level nodes),
  // so children that mount/unmount conditionally (permission dock vs.
  // composer, jump-to-bottom button, todo bar) stay observed.
  useEffect(() => {
    const node = nodeRef.current;
    const observer = resizeObserverRef.current;
    if (!node || !observer) {
      return;
    }
    const current = new Set<Element>(Array.from(node.children));
    for (const el of observedChildrenRef.current) {
      if (!current.has(el)) {
        observer.unobserve(el);
      }
    }
    for (const el of current) {
      if (!observedChildrenRef.current.has(el)) {
        observer.observe(el);
      }
    }
    observedChildrenRef.current = current;
  });

  return {
    containerRef,
    pinned,
    showJumpToBottom,
    jumpToBottom,
    pinAndScrollToBottom,
    handleScroll,
    handleKeyDown,
    handlePointerDown,
  };
}
