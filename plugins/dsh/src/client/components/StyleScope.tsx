import { useEffect, type ReactNode } from 'react';
import { cssText, styles } from 'virtual:flywheel-styles';

let ringAngleRegistered = false;

function ensureRingAngleProperty(): void {
  if (ringAngleRegistered) return;
  ringAngleRegistered = true;
  if (typeof window === 'undefined') return;
  const cssApi = (window as unknown as { CSS?: { registerProperty?: (def: Record<string, unknown>) => void } }).CSS;
  if (typeof cssApi?.registerProperty !== 'function') return;
  try {
    cssApi.registerProperty({
      name: '--fw-ring-angle',
      syntax: '<angle>',
      inherits: false,
      initialValue: '0deg',
    });
  } catch {
    // Already registered via @property stylesheet or prior mount.
  }
}

/**
 * Injects the compiled CSS Modules stylesheet strictly inside the rendered
 * slot subtree, preserving zero import-time side effects on document.head.
 */
export function StyleScope({ children, className }: { children: ReactNode; className?: string }) {
  useEffect(() => {
    ensureRingAngleProperty();
  }, []);

  return (
    <div className={`${styles.root}${className ? ` ${className}` : ''}`} data-flywheel-v2="true">
      <style>{cssText}</style>
      {children}
    </div>
  );
}
