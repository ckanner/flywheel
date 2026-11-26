import { type UIEvent, useCallback, useEffect, useRef, useState } from 'react';

const BOTTOM_THRESHOLD_PX = 64;

/**
 * Chat-grade auto-scroll hook for main stream and node sub-session views:
 * - Jumps to the newest message at the bottom on initial entry and whenever `resetKey`
 *   changes (switching tasks, returning to main flow, switching nodes or invocation rounds).
 * - Keeps pinned to the bottom as async trace details load, streaming deltas arrive, or
 *   inner content height grows, as long as the user has not scrolled up.
 * - Immediately yields control when the user scrolls up and exposes `showScrollToBottom`
 *   so a floating "Jump to latest" button can smoothly return to the bottom.
 */
export function useSmartAutoScroll(resetKey: string, contentKey: string) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const programmaticRef = useRef(false);
  const programmaticTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);

  const performScroll = useCallback((behavior: ScrollBehavior = 'auto') => {
    const el = scrollRef.current;
    if (!el) return;
    if (programmaticTimerRef.current) {
      clearTimeout(programmaticTimerRef.current);
      programmaticTimerRef.current = null;
    }
    programmaticRef.current = true;
    if (behavior === 'smooth' && typeof el.scrollTo === 'function') {
      el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
      programmaticTimerRef.current = setTimeout(() => {
        programmaticRef.current = false;
        programmaticTimerRef.current = null;
      }, 650);
      return;
    }
    el.scrollTop = el.scrollHeight;
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(() => {
        programmaticRef.current = false;
      });
    } else {
      programmaticRef.current = false;
    }
  }, []);

  useEffect(() => {
    return () => {
      if (programmaticTimerRef.current) clearTimeout(programmaticTimerRef.current);
    };
  }, []);

  const scrollToBottom = useCallback(
    (behavior: ScrollBehavior = 'smooth') => {
      stickToBottomRef.current = true;
      setShowScrollToBottom(false);
      performScroll(behavior);
    },
    [performScroll],
  );

  // Reset stickiness and jump straight to bottom whenever switching task/view/node/round.
  useEffect(() => {
    stickToBottomRef.current = true;
    setShowScrollToBottom(false);
    performScroll('auto');
    if (typeof requestAnimationFrame !== 'function') return;
    let raf2 = 0;
    const raf1 = requestAnimationFrame(() => {
      if (stickToBottomRef.current) performScroll('auto');
      raf2 = requestAnimationFrame(() => {
        if (stickToBottomRef.current) performScroll('auto');
      });
    });
    return () => {
      cancelAnimationFrame(raf1);
      if (raf2) cancelAnimationFrame(raf2);
    };
  }, [resetKey, performScroll]);

  // Keep pinned to the latest message while content updates (async trace load / live stream).
  useEffect(() => {
    if (!stickToBottomRef.current) return;
    if (typeof requestAnimationFrame !== 'function') {
      performScroll('auto');
      return;
    }
    const raf = requestAnimationFrame(() => {
      if (stickToBottomRef.current) performScroll('auto');
    });
    return () => cancelAnimationFrame(raf);
  }, [contentKey, performScroll]);

  // Also follow late layout growth (images, async markdown, expanded frames) while pinned.
  useEffect(() => {
    const el = scrollRef.current;
    const inner = el?.firstElementChild;
    if (!el || !inner || typeof ResizeObserver !== 'function') return;
    const ro = new ResizeObserver(() => {
      if (stickToBottomRef.current) {
        performScroll('auto');
      }
    });
    ro.observe(inner);
    return () => ro.disconnect();
  }, [resetKey, performScroll]);

  const handleScroll = useCallback((e: UIEvent<HTMLDivElement>) => {
    if (programmaticRef.current) return;
    const el = e.currentTarget;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    const nearBottom = distance <= BOTTOM_THRESHOLD_PX;
    stickToBottomRef.current = nearBottom;
    setShowScrollToBottom(!nearBottom && el.scrollHeight > el.clientHeight + 120);
  }, []);

  return {
    scrollRef,
    showScrollToBottom,
    scrollToBottom,
    handleScroll,
  };
}
