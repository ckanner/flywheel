import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { ClientRuntime, LayoutService } from '../context.ts';
import type { FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

export interface CommandViewProps {
  store: FlywheelStore;
  t: Translate;
  runtime: ClientRuntime;
  command?: {
    rawInput?: string;
    result?: { kind?: string; text?: string };
  };
  result?: { kind?: string; text?: string };
}

function summarizePayload(text?: string): { action?: string; taskDir?: string; ok?: boolean; message?: string } {
  if (!text) return {};
  try {
    const parsed = JSON.parse(text) as Record<string, unknown>;
    return {
      action: typeof parsed.action === 'string' ? parsed.action : undefined,
      taskDir: typeof parsed.taskDir === 'string' ? parsed.taskDir : undefined,
      ok: typeof parsed.ok === 'boolean' ? parsed.ok : parsed.type !== 'flywheel-control-error',
      message: typeof parsed.message === 'string' ? parsed.message : undefined,
    };
  } catch {
    return {};
  }
}

export function CommandCard({ store, t, runtime, command, result }: CommandViewProps) {
  const effectiveResult = result ?? command?.result;
  const info = summarizePayload(effectiveResult?.text);
  const isError = effectiveResult?.kind === 'error' || info.ok === false;

  const openWorkbench = () => {
    if (info.taskDir) {
      void store.selectTask(info.taskDir);
    }
    const layout = runtime.getService<LayoutService>('layout');
    if (layout?.selectPanel) layout.selectPanel('flywheel');
    else if (layout?.openPanel) layout.openPanel('flywheel');
  };

  return (
    <StyleScope>
      <div className={styles.commandCard} data-testid="flywheel-command-card">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <strong style={{ fontSize: 12.5 }}>{t('cmd.title')}</strong>
            {info.action && <span className={styles.pill}>{info.action}</span>}
            <span className={`${styles.pill} ${isError ? styles.pillDanger : styles.pillSuccess}`}>
              {isError ? t('cmd.status.error') : t('cmd.status.ok')}
            </span>
          </div>
          {(info.taskDir || info.message) && (
            <span style={{ fontSize: 11.5, color: 'var(--fw-text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {info.message || info.taskDir}
            </span>
          )}
        </div>
        <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={openWorkbench}>
          {t('cmd.openWorkbench')}
        </button>
      </div>
    </StyleScope>
  );
}
