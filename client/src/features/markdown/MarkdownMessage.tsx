import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import styles from './MarkdownMessage.module.css';

interface MarkdownMessageProps {
  content?: string;
  role?: string;
  variant?: 'final' | 'process' | 'user';
}

const markdownComponents: Components = {
  a({ node, children, href, ...props }) {
    void node;
    return (
      <a {...props} href={href} target="_blank" rel="noreferrer noopener">
        {children}
      </a>
    );
  },
  code({ node, children, className, ...props }) {
    void node;
    const language = /language-(\S+)/.exec(className ?? '')?.[1];
    return (
      <code {...props} className={[styles.markdownCode, className].filter(Boolean).join(' ')} data-language={language || undefined}>
        {children}
      </code>
    );
  },
  pre({ node, children, ...props }) {
    void node;
    return (
      <pre {...props} className={styles.markdownPre}>
        {children}
      </pre>
    );
  },
  table({ node, children, ...props }) {
    void node;
    return (
      <div className={styles.markdownTableWrap}>
        <table {...props}>{children}</table>
      </div>
    );
  },
};

export function MarkdownMessage({ content = '', role, variant }: MarkdownMessageProps) {
  const resolvedVariant = variant ?? (role === 'user' ? 'user' : 'final');
  return (
    <div
      className={`${styles.markdown} ${resolvedVariant === 'user' ? styles.userMarkdown : resolvedVariant === 'process' ? styles.processMarkdown : styles.finalMarkdown}`}
      data-markdown-variant={resolvedVariant}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
