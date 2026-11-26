import { useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { HtmlPreviewFrame } from '../components/HtmlPreviewFrame.tsx';
import { MarkdownView } from '../components/MarkdownView.tsx';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';

export function PreviewPane({
  snap,
  store,
  isZh,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  isZh: boolean;
}) {
  const [showSource, setShowSource] = useState(false);
  const [copied, setCopied] = useState(false);
  const art = snap.previewArtifact;
  if (!art) return null;

  const isHtml = art.kind === 'html';
  const isMarkdown = art.kind === 'markdown';

  const handleCopyPath = () => {
    const text = art.absPath || art.relPath;
    if (!text) return;
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    });
  };

  return (
    <aside
      className={`${styles.previewPane} ${art.fullscreen ? styles.previewPaneFull : ''}`}
      data-testid="flywheel-preview-pane"
    >
      <header className={styles.previewPaneHead}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, flex: 1 }}>
          <span className={styles.pill}>{isHtml ? 'HTML' : isMarkdown ? 'Markdown' : art.kind.toUpperCase()}</span>
          <strong
            style={{ fontSize: 12.5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
            title={art.absPath || art.relPath}
          >
            {art.title || art.relPath}
          </strong>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
          {(isHtml || isMarkdown) && !art.loading && art.content && (
            <button
              type="button"
              className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
              onClick={() => setShowSource(v => !v)}
            >
              {showSource
                ? (isZh ? '交互预览' : 'Preview')
                : (isZh ? '源码' : 'Source')}
            </button>
          )}
          {art.relPath && (
            <button
              type="button"
              className={`${styles.btn} ${styles.btnSm}`}
              data-testid="preview-open-external"
              onClick={() => void store.openArtifactExternal(art.relPath)}
            >
              {isZh ? (isHtml ? '浏览器打开 ↗' : '系统打开 ↗') : 'Open ↗'}
            </button>
          )}
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            onClick={handleCopyPath}
          >
            {copied ? (isZh ? '已复制 ✓' : 'Copied ✓') : (isZh ? '复制路径' : 'Copy path')}
          </button>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            data-testid="preview-toggle-fullscreen"
            onClick={() => store.togglePreviewFullscreen()}
          >
            {art.fullscreen ? (isZh ? '分栏 ⤡' : 'Split ⤡') : (isZh ? '全屏 ⤢' : 'Full ⤢')}
          </button>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            data-testid="preview-close"
            onClick={() => store.closeArtifactPreview()}
          >
            ✕
          </button>
        </div>
      </header>

      <div className={styles.previewPaneBody}>
        {art.loading ? (
          <div style={{ padding: 24, fontSize: 13, color: 'var(--fw-text-subtle)' }}>
            {isZh ? '正在加载产物内容…' : 'Loading artifact…'}
          </div>
        ) : art.error ? (
          <div className={styles.bannerError} style={{ margin: 16 }}>
            {art.error}
          </div>
        ) : isHtml && !showSource ? (
          <HtmlPreviewFrame
            html={art.content}
            title={art.title || art.relPath}
            className={styles.previewPaneIframe}
            testId="preview-pane-iframe"
          />
        ) : isMarkdown && !showSource ? (
          <div style={{ padding: '18px 22px', overflow: 'auto', flex: 1 }}>
            <MarkdownView content={art.content} />
          </div>
        ) : (
          <pre
            className={styles.monoPre}
            style={{ margin: 0, borderRadius: 0, border: 'none', flex: 1, overflow: 'auto', padding: '14px 18px' }}
          >
            {art.content || '—'}
          </pre>
        )}
      </div>
    </aside>
  );
}
