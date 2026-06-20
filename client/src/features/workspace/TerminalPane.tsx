import { useEffect, useRef } from 'react';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal as XtermTerminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { TerminalEventViewModel, TerminalViewModel } from '../../runtime/workbenchTypes.ts';
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
  const terminalRef = useRef<XtermTerminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const lastSizeRef = useRef({ columns: terminal?.columns ?? 0, rows: terminal?.rows ?? 0 });
  const resizeTimerRef = useRef<number | undefined>(undefined);
  const inputBufferRef = useRef('');
  const inputFlushTimerRef = useRef<number | undefined>(undefined);
  const inputFlushPromiseRef = useRef<Promise<void>>(Promise.resolve());
  const outputWritePromiseRef = useRef<Promise<void>>(Promise.resolve());

  useEffect(() => {
    const host = hostRef.current;
    if (!host || !terminalID) {
      return undefined;
    }

    const xterm = new XtermTerminal({
      allowProposedApi: false,
      convertEol: false,
      cursorBlink: true,
      cursorStyle: 'block',
      disableStdin: false,
      fontFamily: '"Cascadia Mono", Consolas, "SFMono-Regular", monospace',
      fontSize: 13,
      lineHeight: 1.25,
      scrollback: 5000,
      theme: {
        background: '#ffffff',
        foreground: '#111111',
        cursor: '#111111',
        selectionBackground: '#cfe3ff',
        black: '#000000',
        red: '#b42318',
        green: '#008f3a',
        yellow: '#a46f00',
        blue: '#0057c2',
        magenta: '#7a2ce0',
        cyan: '#007a7a',
        white: '#e8e8e8',
        brightBlack: '#5f6368',
        brightRed: '#d92d20',
        brightGreen: '#079455',
        brightYellow: '#b7791f',
        brightBlue: '#1967d2',
        brightMagenta: '#9333ea',
        brightCyan: '#0891b2',
        brightWhite: '#111111',
      },
    });
    const fitAddon = new FitAddon();
    xterm.loadAddon(fitAddon);
    xterm.open(host);
    terminalRef.current = xterm;
    fitAddonRef.current = fitAddon;

    let unsubscribe: (() => void) | undefined;
    let disposed = false;

    const flushInput = () => {
      if (disposed) {
        return;
      }
      const data = inputBufferRef.current;
      inputBufferRef.current = '';
      if (!data) {
        return;
      }
      inputFlushPromiseRef.current = inputFlushPromiseRef.current
        .then(() => onInput(terminalID, data).then(() => undefined))
        .catch(() => undefined);
    };
    const scheduleInputFlush = () => {
      if (inputFlushTimerRef.current) {
        window.clearTimeout(inputFlushTimerRef.current);
      }
      inputFlushTimerRef.current = window.setTimeout(flushInput, 8);
    };
    const writeOutput = (data: string | Uint8Array | Array<string | Uint8Array>, acknowledge?: () => void) => {
      const chunks = Array.isArray(data) ? data : [data];
      const hasData = chunks.some((chunk) => (typeof chunk === 'string' ? chunk.length > 0 : chunk.byteLength > 0));
      if (!hasData && !acknowledge) {
        return;
      }
      outputWritePromiseRef.current = outputWritePromiseRef.current
        .then(
          () =>
            new Promise<void>((resolve) => {
              if (disposed) {
                resolve();
                return;
              }
              if (!hasData) {
                acknowledge?.();
                resolve();
                return;
              }
              writeTerminalChunks(xterm, chunks, () => {
                acknowledge?.();
                resolve();
              });
            }),
        )
        .catch(() => undefined);
    };

    const inputDisposable = xterm.onData((data) => {
      inputBufferRef.current += data;
      scheduleInputFlush();
    });

    void Promise.resolve(
      onSubscribe(terminalID, (event) => {
        if (disposed) {
          return;
        }
        const data = terminalEventOutput(event);
        writeOutput(data, event.acknowledge);
        if (event.final || event.status || typeof event.exitCode === 'number') {
          onTerminalChange({
            ...terminal,
            status: event.status ?? terminal.status,
            exitCode: event.exitCode ?? terminal.exitCode,
          });
        }
      }),
    ).then((dispose) => {
      unsubscribe = dispose;
    });

    const fitAndResize = () => {
      if (disposed) {
        return;
      }
      fitAddon.fit();
      const columns = xterm.cols;
      const rows = xterm.rows;
      const lastSize = lastSizeRef.current;
      if (columns > 0 && rows > 0 && (columns !== lastSize.columns || rows !== lastSize.rows)) {
        lastSizeRef.current = { columns, rows };
        void onResize(terminalID, columns, rows).catch(() => undefined);
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
    xterm.focus();

    return () => {
      flushInput();
      disposed = true;
      if (resizeTimerRef.current) {
        window.clearTimeout(resizeTimerRef.current);
      }
      if (inputFlushTimerRef.current) {
        window.clearTimeout(inputFlushTimerRef.current);
      }
      resizeObserver.disconnect();
      unsubscribe?.();
      inputDisposable.dispose();
      xterm.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
    };
  }, [onInput, onResize, onSubscribe, onTerminalChange, terminal, terminalID]);

  useEffect(() => {
    terminalRef.current?.focus();
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

function writeTerminalChunks(xterm: XtermTerminal, chunks: Array<string | Uint8Array>, done: () => void) {
  const writeNext = (index: number) => {
    if (index >= chunks.length) {
      done();
      return;
    }
    const chunk = chunks[index];
    if (typeof chunk === 'string' ? chunk.length === 0 : chunk.byteLength === 0) {
      writeNext(index + 1);
      return;
    }
    xterm.write(chunk, () => writeNext(index + 1));
  };
  writeNext(0);
}

function decodeBase64Bytes(value: string) {
  try {
    const decoded = window.atob(value);
    const bytes = new Uint8Array(decoded.length);
    for (let index = 0; index < decoded.length; index += 1) {
      bytes[index] = decoded.charCodeAt(index);
    }
    return bytes;
  } catch {
    return new Uint8Array();
  }
}

function terminalEventOutput(event: TerminalEventViewModel) {
  if (event.chunks?.length) {
    const stringChunks = event.chunks.every((chunk) => !chunk.binaryBase64);
    if (stringChunks) {
      return event.chunks.map((chunk) => chunk.data ?? '').join('') || terminalErrorText(event.error);
    }
    return event.chunks.map((chunk) => (chunk.binaryBase64 ? decodeBase64Bytes(chunk.binaryBase64) : (chunk.data ?? '')));
  }
  return event.binaryBase64 ? decodeBase64Bytes(event.binaryBase64) : (event.data ?? terminalErrorText(event.error));
}

function terminalErrorText(error?: string) {
  return error ? `\r\n${error}\r\n` : '';
}
