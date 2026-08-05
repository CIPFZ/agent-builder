import { useEffect, useRef } from 'react';
import '@xterm/xterm/css/xterm.css';
import type { TerminalEventViewModel, TerminalViewModel } from '../../runtime/workbenchTypes.ts';
import {
  attachTerminalRuntime,
  disposeTerminalRuntime,
  flushTerminalInput,
  getTerminalRuntime,
  startTerminalSubscription,
  terminalRuntimeByID,
} from './terminalRuntime.ts';
import styles from './Workspace.module.css';

interface TerminalPaneProps {
  terminal?: TerminalViewModel;
  title: string;
  onInput: (terminalID: string, data: string) => Promise<TerminalViewModel>;
  onResize: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>;
  onSubscribe: (terminalID: string, onEvent: (event: TerminalEventViewModel) => void) => Promise<() => void> | (() => void);
  onTerminalChange: (terminal: TerminalViewModel) => void;
}

export function TerminalPane({ terminal, title, onInput, onResize, onSubscribe, onTerminalChange }: TerminalPaneProps) {
  const terminalID = terminal?.id;
  const hostRef = useRef<HTMLDivElement | null>(null);
  const resizeTimerRef = useRef<number | undefined>(undefined);
  const lastSizeRef = useRef({ columns: terminal?.columns ?? 0, rows: terminal?.rows ?? 0 });

  useEffect(() => {
    const host = hostRef.current;
    if (!host || !terminalID || !terminal) {
      return undefined;
    }

    const runtime = getTerminalRuntime(terminalID, terminal, onInput, onResize, onTerminalChange);
    runtime.terminal = terminal;
    runtime.onInput = onInput;
    runtime.onResize = onResize;
    runtime.onTerminalChange = onTerminalChange;
    attachTerminalRuntime(runtime, host);
    startTerminalSubscription(runtime, onSubscribe);

    const fitAndResize = () => {
      if (runtime.disposed || !runtime.attached) {
        return;
      }
      runtime.fitAddon.fit();
      const columns = runtime.xterm.cols;
      const rows = runtime.xterm.rows;
      const lastSize = lastSizeRef.current;
      if (columns > 0 && rows > 0 && (columns !== lastSize.columns || rows !== lastSize.rows)) {
        lastSizeRef.current = { columns, rows };
        void runtime.onResize(terminalID, columns, rows).catch(() => undefined);
      }
    };

    const scheduleFit = () => {
      if (resizeTimerRef.current) {
        window.clearTimeout(resizeTimerRef.current);
      }
      resizeTimerRef.current = window.setTimeout(fitAndResize, 40);
    };

    const resizeObserver = new ResizeObserver(scheduleFit);
    resizeObserver.observe(host);
    window.setTimeout(fitAndResize, 0);
    runtime.xterm.focus();

    return () => {
      runtime.attached = false;
      flushTerminalInput(runtime);
      if (resizeTimerRef.current) {
        window.clearTimeout(resizeTimerRef.current);
      }
      resizeObserver.disconnect();
    };
  }, [onInput, onResize, onSubscribe, onTerminalChange, terminal, terminalID]);

  useEffect(() => {
    if (!terminalID) {
      return;
    }
    terminalRuntimeByID(terminalID)?.xterm.focus();
  }, [terminalID]);

  useEffect(() => {
    if (!terminalID) return undefined;
    // A backend terminal may continue running across tab and Session switches,
    // but its renderer, input timer, and detailed Wails subscription belong
    // only to the currently mounted pane. Replay reconstructs the renderer
    // when the terminal becomes visible again.
    return () => disposeTerminalRuntime(terminalID);
  }, [terminalID]);

  return (
    <div className={styles.terminalPane} role="tabpanel">
      {terminal ? (
        <div ref={hostRef} className={styles.terminalScreen} role="application" aria-label={title} />
      ) : (
        <div className={styles.terminalScreen}>
          <div className={styles.terminalHistoryLine}>Connecting terminal...</div>
        </div>
      )}
    </div>
  );
}
