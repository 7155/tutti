#!/usr/bin/env node
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { recordScenarioDefinitions } from "./agent-session-replay-record-scenarios/definitions.mjs";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const workspaceRoot = resolve(scriptDirectory, "..", "..");
const defaultTimeoutMs = 300_000;

export function recordCaseCommandPlan(
  scenarioId,
  timeoutMs = defaultTimeoutMs
) {
  const scenario = recordScenarioDefinitions[scenarioId];
  if (!scenario) {
    throw new Error(`unsupported record scenario: ${scenarioId}`);
  }
  const cassetteDirectory = `.tmp/cassettes/${scenario.cassetteName}`;
  return {
    cassetteDirectory,
    commands: [
      {
        command: "pnpm",
        args: [
          "e2e:agent-gui",
          "--",
          "--record",
          cassetteDirectory,
          "--scenario",
          scenarioId,
          "--agent-target-id",
          "local:codex",
          "--headless",
          "--keep-runtime",
          "--timeout-ms",
          String(timeoutMs)
        ]
      },
      {
        command: "node",
        args: [
          ".codex/skills/tutti-record-agent-session-replay/scripts/audit-cassette.mjs",
          cassetteDirectory
        ]
      },
      {
        command: "pnpm",
        args: [
          "e2e:agent-gui",
          "--",
          "--replay",
          cassetteDirectory,
          "--headless",
          "--keep-runtime",
          "--timeout-ms",
          String(timeoutMs)
        ]
      }
    ]
  };
}

export async function runRecordCase(scenarioId) {
  const plan = recordCaseCommandPlan(scenarioId);
  for (const command of plan.commands) {
    await runCommand(command.command, command.args);
  }
  process.stdout.write(
    `[agent-session-replay] record, audit, and replay passed: ${plan.cassetteDirectory}\n`
  );
}

export function recordScenarioIdFromArgs(args) {
  if (args.length !== 1 || !args[0]) {
    throw new Error("usage: pnpm record:agent-gui <scenario-id>");
  }
  return args[0];
}

function runCommand(command, args) {
  return new Promise((resolveCommand, rejectCommand) => {
    const child = spawn(command, args, {
      cwd: workspaceRoot,
      env: process.env,
      stdio: "inherit"
    });
    child.once("error", rejectCommand);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolveCommand();
        return;
      }
      rejectCommand(
        new Error(
          `${command} ${args.join(" ")} failed with ${signal ?? `exit ${code}`}`
        )
      );
    });
  });
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  await runRecordCase(recordScenarioIdFromArgs(process.argv.slice(2)));
}
