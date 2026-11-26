import { useEffect, useRef, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { ClientRuntime, UIWorkspaceService } from '../context.ts';
import { formatModelBadge } from '../planning.ts';
import type { FlywheelStore } from '../store.ts';
import type { TraceDetail, TraceSummary, Translate } from '../types.ts';

export async function copyToClipboard(text: string): Promise<boolean> {
  if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fallback below.
    }
  }
  if (typeof document !== 'undefined') {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }
  return false;
}

export function shortSessionId(sessionId?: string): string {
  if (!sessionId) return '';
  if (sessionId.length <= 12) return sessionId;
  return sessionId.slice(0, 8);
}

export function resolveSessionName(trace: TraceSummary | TraceDetail): string {
  if (trace.sessionTitle && trace.sessionTitle.trim()) return trace.sessionTitle.trim();
  const scopePart = trace.scope ? ` · ${trace.scope}` : '';
  return `Flywheel · ${trace.role}${scopePart}`;
}

function isMacDesktop(): boolean {
  if (typeof navigator === 'undefined') return true;
  return /mac/i.test(navigator.platform || navigator.userAgent || '');
}

export function SessionPopoverButton({
  trace,
  store,
  t,
  runtime,
}: {
  trace: TraceSummary | TraceDetail | null;
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const [open, setOpen] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onOutside = (event: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onOutside);
    return () => document.removeEventListener('mousedown', onOutside);
  }, [open]);

  if (!trace) return null;

  const resume = trace.resume ?? null;
  const providerLabel = resume?.providerLabel || trace.harness || 'Provider';
  const sessionId = resume?.sessionId || trace.sessionId || '';
  const shortId = shortSessionId(sessionId);
  const sessionName = resolveSessionName(trace);
  const cwd = resume?.cwd || trace.cwd || '';
  const openTerminalLabel = isMacDesktop() ? t('chat.session.openWarp') : t('chat.session.openTerminal');

  const handleCopy = async (key: string, value: string) => {
    await copyToClipboard(value);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(prev => (prev === key ? null : prev)), 2000);
  };

  const handleOpenDsh = () => {
    if (!sessionId) return;
    const workspace = runtime?.getService<UIWorkspaceService>('uiWorkspace');
    workspace?.openSession?.(sessionId);
    setOpen(false);
  };

  const handleOpenWarp = () => {
    if (!resume) return;
    void store.openResumeTerminal(
      resume.harness,
      resume.sessionId,
      cwd || undefined,
      sessionName,
      isMacDesktop() ? 'warp' : 'terminal',
    );
    setOpen(false);
  };

  return (
    <div className={styles.sessionCapsuleWrap} ref={wrapRef}>
      <button
        type="button"
        className={`${styles.btn} ${styles.btnSm}`}
        data-testid="flywheel-session-capsule"
        aria-expanded={open}
        onClick={() => setOpen(v => !v)}
      >
        <span>{providerLabel}</span>
        {shortId && <code style={{ fontFamily: 'var(--fw-mono)', fontSize: 11, color: 'var(--fw-text-muted)' }}>{shortId}</code>}
        <span>↗</span>
      </button>

      {open && (
        <div
          className={styles.sessionPopover}
          data-testid="flywheel-session-popover"
          role="dialog"
          aria-label={t('chat.session.popoverTitle')}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <strong style={{ fontSize: 12.5 }}>{t('chat.session.popoverTitle')}</strong>
            <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
              <span className={styles.pill}>{providerLabel}</span>
              {trace.model && (
                <span className={`${styles.pill} ${styles.pillPurple}`} title={trace.model}>
                  {formatModelBadge(trace.model, trace.thinking, t('tab.traces') === '运行记录')}
                </span>
              )}
            </div>
          </div>

          <div className={styles.sessionPopField}>
            <span className={styles.sessionPopLabel}>{t('chat.session.name')}</span>
            <div className={styles.sessionPopValue}>
              <span style={{ fontFamily: 'var(--fw-font)', fontSize: 11.5 }}>{sessionName}</span>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                onClick={() => void handleCopy('name', sessionName)}
              >
                {copiedKey === 'name' ? t('chat.session.copied') : t('chat.session.copyId')}
              </button>
            </div>
          </div>

          {sessionId ? (
            <div className={styles.sessionPopField}>
              <span className={styles.sessionPopLabel}>{t('chat.session.id')}</span>
              <div className={styles.sessionPopValue}>
                <code data-testid="session-popover-id">{sessionId}</code>
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                  onClick={() => void handleCopy('id', sessionId)}
                >
                  {copiedKey === 'id' ? t('chat.session.copied') : t('chat.session.copyId')}
                </button>
              </div>
            </div>
          ) : (
            <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>
              {t('traces.noSessionId')}
            </div>
          )}

          {cwd && (
            <div className={styles.sessionPopField}>
              <span className={styles.sessionPopLabel}>{t('chat.session.cwd')}</span>
              <div className={styles.sessionPopValue}>
                <span>{cwd}</span>
              </div>
            </div>
          )}

          {resume?.command && (
            <div className={styles.sessionPopField}>
              <span className={styles.sessionPopLabel}>{t('chat.session.cli')}</span>
              <div className={styles.sessionPopValue} data-testid="session-popover-cmd">
                <code>{resume.command}</code>
              </div>
            </div>
          )}

          {trace.running && (
            <div style={{ fontSize: 11.5, color: 'var(--fw-warn)', lineHeight: 1.45 }}>
              {t('traces.runningWarn')}
            </div>
          )}

          {resume && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', paddingTop: 2 }}>
              {resume.kind === 'dsh-native' ? (
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                  data-testid="session-open-dsh"
                  onClick={handleOpenDsh}
                >
                  {t('chat.session.openDsh')}
                </button>
              ) : (
                <>
                  {resume.command && (
                    <>
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnSm}`}
                        data-testid="session-copy-cli"
                        onClick={() => void handleCopy('cli', resume.command!)}
                      >
                        {copiedKey === 'cli' ? t('chat.session.copied') : t('chat.session.copyCli')}
                      </button>
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                        data-testid="session-open-warp"
                        onClick={handleOpenWarp}
                      >
                        {openTerminalLabel}
                      </button>
                    </>
                  )}
                  {resume.secondaryCommand && (
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnSm}`}
                      onClick={() => void handleCopy('vis', resume.secondaryCommand!)}
                    >
                      {copiedKey === 'vis' ? t('chat.session.copied') : t('traces.copyVis')}
                    </button>
                  )}
                </>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function SessionTopStrip({
  trace,
  store,
  t,
  runtime,
}: {
  trace: TraceSummary | TraceDetail;
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const [copied, setCopied] = useState(false);
  const resume = trace.resume ?? null;
  const providerLabel = resume?.providerLabel || trace.harness || 'Provider';
  const sessionId = resume?.sessionId || trace.sessionId || '';
  const sessionName = resolveSessionName(trace);
  const cwd = resume?.cwd || trace.cwd || '';
  const workspaceCwd = trace.workspaceCwd || '';
  const isWorktree = Boolean(cwd && workspaceCwd && cwd !== workspaceCwd);
  const isZh = t('tab.traces') === '运行记录';
  const openTerminalLabel = isMacDesktop() ? t('chat.session.openWarp') : t('chat.session.openTerminal');
  const cliHint = isZh
    ? '复制原生终端恢复命令（只读查看随时安全；运行中如需发消息请先点右上角「打断接管」）'
    : 'Copy native CLI resume command (safe to inspect read-only; use Interrupt & Take Over before sending while running)';

  const handleCopyCmd = async () => {
    const val = resume?.command || sessionId;
    if (!val) return;
    await copyToClipboard(val);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleOpen = () => {
    if (!resume) return;
    if (resume.kind === 'dsh-native') {
      const workspace = runtime?.getService<UIWorkspaceService>('uiWorkspace');
      workspace?.openSession?.(resume.sessionId);
      return;
    }
    void store.openResumeTerminal(
      resume.harness,
      resume.sessionId,
      cwd || undefined,
      sessionName,
      isMacDesktop() ? 'warp' : 'terminal',
    );
  };

  return (
    <div className={styles.sessionTopStrip} data-testid="flywheel-session-strip">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', minWidth: 0 }}>
        <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--fw-text-subtle)', whiteSpace: 'nowrap' }}>
          {isZh ? '底层原生会话' : 'Native Session'}
        </span>
        <span className={styles.pill}>{providerLabel}</span>
        {trace.model && (
          <span className={`${styles.pill} ${styles.pillPurple}`} title={trace.model}>
            {formatModelBadge(trace.model, trace.thinking, isZh)}
          </span>
        )}
        {cwd && (
          <span
            className={styles.pill}
            title={isWorktree ? `${isZh ? '会话归属主仓库' : 'Session repo'}: ${workspaceCwd}\n${isZh ? '执行目录 (worktree)' : 'Worktree'}: ${trace.cwd}` : cwd}
            style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5 }}
          >
            {isWorktree
              ? (isZh ? '会话: 主仓库 · 执行: worktree' : 'Session: repo · Work: worktree')
              : (isZh ? '会话: 主仓库' : 'Session: repo')}
          </span>
        )}
        <span
          title={sessionName}
          style={{ color: 'var(--fw-text-muted)', fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        >
          {sessionName}
        </span>
        {sessionId && (
          <code
            title={sessionId}
            style={{ fontFamily: 'var(--fw-mono)', fontSize: 11, color: 'var(--fw-text-subtle)' }}
          >
            #{shortSessionId(sessionId)}
          </code>
        )}
      </div>
      {resume && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
          {(resume.command || sessionId) && (
            <button
              type="button"
              className={`${styles.btn} ${styles.btnSm}`}
              title={cliHint}
              onClick={() => void handleCopyCmd()}
            >
              {copied ? t('chat.session.copied') : t('chat.session.copyCli')}
            </button>
          )}
          <button
            type="button"
            className={`${styles.btn} ${styles.btnSm}`}
            title={cliHint}
            onClick={handleOpen}
          >
            {resume.kind === 'dsh-native' ? t('chat.session.openDsh') : openTerminalLabel}
          </button>
        </div>
      )}
    </div>
  );
}
