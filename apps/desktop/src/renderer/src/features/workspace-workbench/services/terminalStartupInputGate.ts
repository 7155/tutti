import type { TerminalTransport } from "@tutti-os/workspace-terminal/contracts";

export type TerminalStartupInputResult =
  | "cancelled"
  | "submitted"
  | "timed_out"
  | "write_failed";

export interface TerminalStartupInputGate {
  arm(sessionId: string): Promise<TerminalStartupInputResult>;
  cancel(): void;
}

const defaultReadyTimeoutMs = 15_000;
const maxBufferedOutputChars = 4_096;
const maxBufferedSessions = 8;

export function createTerminalStartupInputGate(input: {
  readyText: string;
  startupInput: string;
  timeoutMs?: number;
  transport: Pick<TerminalTransport, "onData" | "write">;
}): TerminalStartupInputGate {
  const readyText = input.readyText.trim();
  const startupInput = input.startupInput.trim();
  const bufferedOutputBySession = new Map<string, string>();
  let armedSessionId: string | null = null;
  let finished = false;
  let submitting = false;
  let timeout: ReturnType<typeof setTimeout> | null = null;
  let unsubscribe = noop;
  let resolveCompletion: (result: TerminalStartupInputResult) => void = noop;
  const completion = new Promise<TerminalStartupInputResult>((resolve) => {
    resolveCompletion = resolve;
  });

  const finish = (result: TerminalStartupInputResult) => {
    if (finished) return;
    finished = true;
    if (timeout) clearTimeout(timeout);
    timeout = null;
    unsubscribe();
    resolveCompletion(result);
  };

  const maybeSubmit = () => {
    if (finished || submitting || !armedSessionId) return;
    const output = bufferedOutputBySession.get(armedSessionId) ?? "";
    if (!output.includes(readyText)) return;
    submitting = true;
    void input.transport
      .write({
        data: terminalSubmitInput(startupInput),
        encoding: "utf8",
        provenance: "auto",
        sessionId: armedSessionId
      })
      .then(
        () => finish("submitted"),
        () => finish("write_failed")
      );
  };

  unsubscribe = input.transport.onData((event) => {
    const current = bufferedOutputBySession.get(event.sessionId) ?? "";
    bufferedOutputBySession.set(
      event.sessionId,
      `${current}${event.data}`.slice(-maxBufferedOutputChars)
    );
    while (bufferedOutputBySession.size > maxBufferedSessions) {
      const oldestSessionId = bufferedOutputBySession.keys().next().value;
      if (typeof oldestSessionId !== "string") break;
      bufferedOutputBySession.delete(oldestSessionId);
    }
    maybeSubmit();
  });

  return {
    arm(sessionId) {
      if (armedSessionId || finished) return completion;
      armedSessionId = sessionId.trim();
      if (!armedSessionId || !readyText || !startupInput) {
        finish("cancelled");
        return completion;
      }
      timeout = setTimeout(
        () => finish("timed_out"),
        input.timeoutMs ?? defaultReadyTimeoutMs
      );
      maybeSubmit();
      return completion;
    },
    cancel() {
      finish("cancelled");
    }
  };
}

function terminalSubmitInput(input: string): string {
  return /[\r\n]$/u.test(input) ? input : `${input}\r`;
}

function noop(): void {}
