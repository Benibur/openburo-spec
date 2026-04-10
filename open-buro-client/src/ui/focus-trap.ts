// src/ui/focus-trap.ts

const FOCUSABLE_SELECTORS = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[role="option"][tabindex="0"]',
].join(',');

/**
 * Installs a focus trap inside `container` queried from `root` (ShadowRoot).
 * Returns a cleanup function that removes the trap.
 *
 * WCAG 2.1 SC 2.1.2 compliance: focus cannot leave the modal while open.
 * Uses root.querySelectorAll (not document.querySelectorAll) so focusable
 * elements inside the shadow boundary are correctly found.
 * Uses root.activeElement (not document.activeElement) because the latter
 * returns the shadow host, not the focused element inside the shadow tree.
 */
export function trapFocus(root: ShadowRoot, container: HTMLElement): () => void {
  function getFocusableElements(): HTMLElement[] {
    return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)).filter((el) =>
      container.contains(el),
    );
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (e.key !== 'Tab') return;

    const focusable = getFocusableElements();
    if (focusable.length === 0) {
      e.preventDefault();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = root.activeElement;

    if (e.shiftKey) {
      if (active === first || active === container) {
        e.preventDefault();
        last?.focus();
      }
    } else {
      if (active === last) {
        e.preventDefault();
        first?.focus();
      }
    }
  }

  root.addEventListener('keydown', onKeyDown as EventListener);

  const firstFocusable = getFocusableElements()[0];
  requestAnimationFrame(() => {
    firstFocusable?.focus();
  });

  return () => {
    root.removeEventListener('keydown', onKeyDown as EventListener);
  };
}
