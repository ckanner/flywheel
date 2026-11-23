import type { ReactNode } from 'react';
import { cssText, styles } from 'virtual:flywheel-styles';

/**
 * Injects the compiled CSS Modules stylesheet strictly inside the rendered
 * slot subtree, preserving zero import-time side effects on document.head.
 */
export function StyleScope({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={`${styles.root}${className ? ` ${className}` : ''}`} data-flywheel-v2="true">
      <style>{cssText}</style>
      {children}
    </div>
  );
}
