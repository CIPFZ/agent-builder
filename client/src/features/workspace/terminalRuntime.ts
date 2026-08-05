import { FitAddon } from '@xterm/addon-fit';
import { Terminal as XtermTerminal } from '@xterm/xterm';
import type { IDisposable, ITheme } from '@xterm/xterm';
import type { TerminalEventViewModel, TerminalViewModel } from '../../runtime/workbenchTypes.ts';

export interface TerminalRuntime {
  id: string;
  xterm: XtermTerminal;
  fitAddon: FitAddon;
  inputDisposable: IDisposable;
  terminal?: TerminalViewModel;
  attached: boolean;
  disposed: boolean;
  unsubscribe?: () => void;
  subscriptionStarted: boolean;
  inputBuffer: string;
  inputFlushTimer?: number;
  inputFlushPromise: Promise<void>;
  outputWritePromise: Promise<void>;
  onInput: (terminalID: string, data: string) => Promise<TerminalViewModel>;
  onResize: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>;
  onTerminalChange: (terminal: TerminalViewModel) => void;
}

const terminalRuntimes = new Map<string, TerminalRuntime>();

const terminalANSIThemes: Record<'light' | 'dark', ITheme> = {
  light: {
    black: '#000000', red: '#b42318', green: '#008f3a', yellow: '#a46f00', blue: '#0057c2', magenta: '#7a2ce0', cyan: '#007a7a', white: '#e8e8e8',
    brightBlack: '#5f6368', brightRed: '#d92d20', brightGreen: '#079455', brightYellow: '#b7791f', brightBlue: '#1967d2', brightMagenta: '#9333ea', brightCyan: '#0891b2', brightWhite: '#111111',
  },
  dark: {
    black: '#1f1f1f', red: '#ff7875', green: '#73d13d', yellow: '#ffc53d', blue: '#69b1ff', magenta: '#b37feb', cyan: '#5cdbd3', white: '#d9d9d9',
    brightBlack: '#8c8c8c', brightRed: '#ffa39e', brightGreen: '#95de64', brightYellow: '#ffd666', brightBlue: '#91caff', brightMagenta: '#d3adf7', brightCyan: '#87e8de', brightWhite: '#f5f5f5',
  },
};

function terminalTheme(): ITheme {
  const mode = document.documentElement.dataset.colorMode === 'dark' ? 'dark' : 'light';
  const style = getComputedStyle(document.documentElement);
  return {
    ...terminalANSIThemes[mode],
    background: style.getPropertyValue('--app-surface-panel').trim(),
    foreground: style.getPropertyValue('--app-text-primary').trim(),
    cursor: style.getPropertyValue('--app-text-primary').trim(),
    selectionBackground: style.getPropertyValue('--app-surface-active').trim(),
  };
}

window.addEventListener('app-theme-change', () => {
  const nextTheme = terminalTheme();
  for (const runtime of terminalRuntimes.values()) {
    if (!runtime.disposed) runtime.xterm.options.theme = nextTheme;
  }
});

export function terminalRuntimeByID(terminalID: string) {
  return terminalRuntimes.get(terminalID);
}

export function disposeTerminalRuntime(terminalID: string) {
  const runtime = terminalRuntimes.get(terminalID);
  if (!runtime) {
    return;
  }
  terminalRuntimes.delete(terminalID);
  runtime.disposed = true;
  if (runtime.inputFlushTimer) {
    window.clearTimeout(runtime.inputFlushTimer);
  }
  flushTerminalInput(runtime);
  runtime.unsubscribe?.();
  runtime.inputDisposable.dispose();
  runtime.xterm.dispose();
}

export function getTerminalRuntime(
  terminalID: string,
  terminal: TerminalViewModel,
  onInput: (terminalID: string, data: string) => Promise<TerminalViewModel>,
  onResize: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>,
  onTerminalChange: (terminal: TerminalViewModel) => void,
) {
  const existing = terminalRuntimes.get(terminalID);
  if (existing && !existing.disposed) {
    return existing;
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
    theme: terminalTheme(),
  });
  const fitAddon = new FitAddon();
  xterm.loadAddon(fitAddon);

  const runtime: TerminalRuntime = {
    id: terminalID,
    xterm,
    fitAddon,
    inputDisposable: { dispose: () => undefined },
    terminal,
    attached: false,
    disposed: false,
    subscriptionStarted: false,
    inputBuffer: '',
    inputFlushPromise: Promise.resolve(),
    outputWritePromise: Promise.resolve(),
    onInput,
    onResize,
    onTerminalChange,
  };
  runtime.inputDisposable = xterm.onData((data) => {
    runtime.inputBuffer += data;
    scheduleInputFlush(runtime);
  });
  terminalRuntimes.set(terminalID, runtime);
  return runtime;
}

export function attachTerminalRuntime(runtime: TerminalRuntime, host: HTMLDivElement) {
  runtime.attached = true;
  if (runtime.xterm.element) {
    host.appendChild(runtime.xterm.element);
    return;
  }
  runtime.xterm.open(host);
}

export function startTerminalSubscription(
  runtime: TerminalRuntime,
  onSubscribe: (terminalID: string, onEvent: (event: TerminalEventViewModel) => void) => Promise<() => void> | (() => void),
) {
  if (runtime.subscriptionStarted || runtime.disposed) {
    return;
  }
  runtime.subscriptionStarted = true;
  void Promise.resolve(
    onSubscribe(runtime.id, (event) => {
      if (runtime.disposed) {
        return;
      }
      writeOutput(runtime, terminalEventOutput(event), event.acknowledge);
      if (event.final || event.status || typeof event.exitCode === 'number') {
        const terminal = runtime.terminal;
        if (terminal) {
          const nextTerminal = {
            ...terminal,
            status: event.status ?? terminal.status,
            exitCode: event.exitCode ?? terminal.exitCode,
          };
          runtime.terminal = nextTerminal;
          runtime.onTerminalChange(nextTerminal);
        }
      }
    }),
  )
    .then((dispose) => {
      if (runtime.disposed) {
        dispose();
        return;
      }
      runtime.unsubscribe = dispose;
    })
    .catch((error) => {
      runtime.subscriptionStarted = false;
      writeOutput(runtime, terminalErrorText(error instanceof Error ? error.message : 'Terminal stream failed'));
    });
}

export function flushTerminalInput(runtime: TerminalRuntime) {
  if (runtime.disposed && !runtime.inputBuffer) {
    return;
  }
  const data = runtime.inputBuffer;
  runtime.inputBuffer = '';
  if (!data) {
    return;
  }
  runtime.inputFlushPromise = runtime.inputFlushPromise
    .then(() => runtime.onInput(runtime.id, data).then(() => undefined))
    .catch(() => undefined);
}

function scheduleInputFlush(runtime: TerminalRuntime) {
  if (runtime.inputFlushTimer) {
    window.clearTimeout(runtime.inputFlushTimer);
  }
  runtime.inputFlushTimer = window.setTimeout(() => flushTerminalInput(runtime), 8);
}

function writeOutput(runtime: TerminalRuntime, data: string | Uint8Array | Array<string | Uint8Array>, acknowledge?: () => void) {
  const chunks = Array.isArray(data) ? data : [data];
  const hasData = chunks.some((chunk) => (typeof chunk === 'string' ? chunk.length > 0 : chunk.byteLength > 0));
  if (!hasData && !acknowledge) {
    return;
  }
  runtime.outputWritePromise = runtime.outputWritePromise
    .then(
      () =>
        new Promise<void>((resolve) => {
          if (runtime.disposed) {
            resolve();
            return;
          }
          if (!hasData) {
            acknowledge?.();
            resolve();
            return;
          }
          writeTerminalChunks(runtime.xterm, chunks, () => {
            acknowledge?.();
            resolve();
          });
        }),
    )
    .catch(() => undefined);
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
