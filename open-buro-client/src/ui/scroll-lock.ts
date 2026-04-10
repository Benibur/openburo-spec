// src/ui/scroll-lock.ts

/**
 * IFR-10: lock body scroll while modal/iframe is open.
 * Returns a restorer function that reverts to the previous overflow value
 * on every close path.
 */
export function lockBodyScroll(): () => void {
  const prev = document.body.style.overflow;
  document.body.style.overflow = 'hidden';
  return () => {
    document.body.style.overflow = prev;
  };
}
