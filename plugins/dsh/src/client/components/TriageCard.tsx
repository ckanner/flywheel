import type { ReactNode } from 'react';
import { styles } from 'virtual:flywheel-styles';

export interface TriageCardProps {
  testId?: string;
  tone?: 'warn' | 'danger' | 'neutral';
  headerLeft: ReactNode;
  headerRight?: ReactNode;
  body?: ReactNode;
  noteInput?: ReactNode;
  actions?: ReactNode;
  error?: ReactNode;
}

/**
 * Shared human-triage card enforcing consistent inner padding, border rhythm,
 * and natural form order: header -> reason/findings -> optional note input -> actions.
 */
export function TriageCard({
  testId,
  tone = 'warn',
  headerLeft,
  headerRight,
  body,
  noteInput,
  actions,
  error,
}: TriageCardProps) {
  const toneClass =
    tone === 'danger'
      ? styles.decisionCardDanger
      : tone === 'warn'
        ? styles.decisionCardWarn
        : '';

  return (
    <div className={`${styles.decisionCard} ${toneClass}`} data-testid={testId}>
      <div className={styles.decisionCardHeader}>
        <div className={styles.decisionCardTitleGroup}>{headerLeft}</div>
        {headerRight ? <div className={styles.decisionCardHeaderRight}>{headerRight}</div> : null}
      </div>
      {body}
      {noteInput}
      {actions ? <div className={styles.decisionCardActions}>{actions}</div> : null}
      {error}
    </div>
  );
}
