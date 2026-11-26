import { isValidElement, useCallback, useMemo, useState } from 'react';
import { Streamdown, type Components } from 'streamdown';
import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import dockerfile from 'highlight.js/lib/languages/dockerfile';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import markdown from 'highlight.js/lib/languages/markdown';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';
import { styles } from 'virtual:flywheel-styles';

hljs.registerLanguage('bash', bash);
hljs.registerLanguage('sh', bash);
hljs.registerLanguage('shell', bash);
hljs.registerLanguage('zsh', bash);
hljs.registerLanguage('css', css);
hljs.registerLanguage('diff', diff);
hljs.registerLanguage('dockerfile', dockerfile);
hljs.registerLanguage('go', go);
hljs.registerLanguage('golang', go);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('js', javascript);
hljs.registerLanguage('jsx', javascript);
hljs.registerLanguage('json', json);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('md', markdown);
hljs.registerLanguage('python', python);
hljs.registerLanguage('py', python);
hljs.registerLanguage('rust', rust);
hljs.registerLanguage('rs', rust);
hljs.registerLanguage('sql', sql);
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('ts', typescript);
hljs.registerLanguage('tsx', typescript);
hljs.registerLanguage('xml', xml);
hljs.registerLanguage('html', xml);
hljs.registerLanguage('svg', xml);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('yml', yaml);

function extractText(node: unknown): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (isValidElement<{ children?: unknown }>(node)) {
    return extractText(node.props?.children);
  }
  return '';
}

function CodeBlockCard({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);

  const highlighted = useMemo(() => {
    const lang = language.toLowerCase();
    try {
      if (lang && hljs.getLanguage(lang)) {
        return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
      }
    } catch {
      // Fallback to plain text rendering.
    }
    return null;
  }, [code, language]);

  const handleCopy = useCallback(() => {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      void navigator.clipboard.writeText(code).then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1600);
      });
    }
  }, [code]);

  return (
    <div className={styles.codeCard}>
      <div className={styles.codeCardHead}>
        <span className={styles.codeCardLang}>{language || 'text'}</span>
        <button
          type="button"
          className={styles.codeCardCopyBtn}
          onClick={handleCopy}
        >
          {copied ? '✓ 已复制' : '复制'}
        </button>
      </div>
      <pre className={styles.codeCardPre}>
        {highlighted !== null ? (
          <code className="hljs" dangerouslySetInnerHTML={{ __html: highlighted }} />
        ) : (
          <code>{code}</code>
        )}
      </pre>
    </div>
  );
}

const STREAMDOWN_COMPONENTS: Components = {
  pre({ children }) {
    const child = Array.isArray(children) ? children[0] : children;
    if (isValidElement<{ className?: string; children?: unknown }>(child)) {
      const cls = typeof child.props?.className === 'string' ? child.props.className : '';
      const langMatch = /language-([A-Za-z0-9_+-]+)/.exec(cls);
      const language = langMatch?.[1] ?? '';
      const rawCode = extractText(child.props?.children).replace(/\n$/, '');
      return <CodeBlockCard code={rawCode} language={language} />;
    }
    return <pre className={styles.codeCardPre}>{children}</pre>;
  },
  code({ className, children, ...rest }) {
    const isFenced = typeof className === 'string' && className.includes('language-');
    if (isFenced) {
      return (
        <code className={className} {...rest}>
          {children}
        </code>
      );
    }
    return (
      <code className={styles.inlineCode} {...rest}>
        {children}
      </code>
    );
  },
  a({ href, children, ...rest }) {
    const safeHref = typeof href === 'string' && /^(https?:|mailto:|#)/i.test(href) ? href : undefined;
    return (
      <a href={safeHref} target={safeHref?.startsWith('http') ? '_blank' : undefined} rel="noreferrer" {...rest}>
        {children}
      </a>
    );
  },
  table({ children, ...rest }) {
    return (
      <div style={{ overflowX: 'auto', margin: '10px 0' }}>
        <table {...rest}>{children}</table>
      </div>
    );
  },
};

export function MarkdownView({
  content,
  className,
  streaming = false,
}: {
  content: string;
  className?: string;
  streaming?: boolean;
}) {
  const source = typeof content === 'string' ? content : '';
  return (
    <div className={`${styles.markdown}${className ? ` ${className}` : ''}`}>
      <Streamdown
        mode={streaming ? 'streaming' : 'static'}
        isAnimating={streaming}
        parseIncompleteMarkdown={streaming}
        controls={false}
        linkSafety={{ enabled: false }}
        components={STREAMDOWN_COMPONENTS}
      >
        {source}
      </Streamdown>
    </div>
  );
}
