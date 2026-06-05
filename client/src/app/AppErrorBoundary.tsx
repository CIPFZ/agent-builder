import { Component, type ErrorInfo, type ReactNode } from 'react';
import styles from './AppErrorBoundary.module.css';

interface AppErrorBoundaryProps {
  children: ReactNode;
}

interface AppErrorBoundaryState {
  error?: Error;
}

export class AppErrorBoundary extends Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  state: AppErrorBoundaryState = {};

  static getDerivedStateFromError(error: Error): AppErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('[app] unhandled render error', error, errorInfo);
  }

  render() {
    const { error } = this.state;
    if (error) {
      return (
        <main className={styles.fallback} role="alert">
          <section className={styles.panel}>
            <h1>应用启动失败</h1>
            <p>前端渲染过程中发生异常，请查看开发者控制台或桌面日志。</p>
            <pre>{error.message || String(error)}</pre>
          </section>
        </main>
      );
    }

    return this.props.children;
  }
}
