import { useEffect, useState } from 'react';

/**
 * Renders a self-contained HTML document via a stable object Blob URL (matching
 * DSH's native HtmlBody preview pattern) so parent React polling re-renders
 * never reset the iframe browsing context or interrupt interactive clicks.
 */
export function HtmlPreviewFrame({
  html,
  title = 'HTML Interactive Preview',
  className,
  style,
  testId,
}: {
  html: string;
  title?: string;
  className?: string;
  style?: React.CSSProperties;
  testId?: string;
}) {
  const [blobUrl, setBlobUrl] = useState<string>('');

  useEffect(() => {
    if (!html) {
      setBlobUrl('');
      return;
    }
    const url = URL.createObjectURL(new Blob([html], { type: 'text/html;charset=utf-8' }));
    setBlobUrl(url);
    return () => {
      URL.revokeObjectURL(url);
    };
  }, [html]);

  if (!blobUrl) return null;

  return (
    <iframe
      key={blobUrl}
      title={title}
      src={blobUrl}
      sandbox="allow-scripts allow-forms allow-modals allow-popups"
      className={className}
      style={style}
      data-testid={testId}
    />
  );
}
