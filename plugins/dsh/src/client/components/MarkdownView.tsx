import { useMemo } from 'react';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { styles } from 'virtual:flywheel-styles';

marked.setOptions({
  gfm: true,
  breaks: false,
});

function renderSafeMarkdown(content: string): string {
  const rawHtml = marked.parse(content, { async: false });
  if (typeof rawHtml !== 'string') return '';
  if (typeof window !== 'undefined' && typeof DOMPurify?.sanitize === 'function') {
    return DOMPurify.sanitize(rawHtml, {
      USE_PROFILES: { html: true },
      FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'form'],
    });
  }
  return rawHtml;
}

export function MarkdownView({ content, className }: { content: string; className?: string }) {
  const html = useMemo(() => renderSafeMarkdown(content || ''), [content]);
  return (
    <div
      className={`${styles.markdown}${className ? ` ${className}` : ''}`}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
