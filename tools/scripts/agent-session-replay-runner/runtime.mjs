import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { access, mkdir, mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { buildDaemon, stopProcessTree } from "../run-agent-gui-performance.mjs";

export const replayListenerInfoPath = (stateDirectory) =>
  join(stateDirectory, "run", "tuttid.listener.json");

export async function createRuntime(workspaceRoot, mode) {
  const runtimeParent =
    process.env.TUTTI_AGENT_SESSION_REPLAY_RUNTIME_PARENT?.trim() ||
    join(workspaceRoot, ".tmp");
  await mkdir(runtimeParent, { recursive: true });
  const directory = await mkdtemp(
    join(runtimeParent, `agent-session-${mode}-`)
  );
  const stateDirectory = join(directory, "state");
  const userDataDirectory = join(directory, "electron-user-data");
  const daemonPath = join(directory, "tuttid");
  await mkdir(stateDirectory, { recursive: true });
  const preparedDaemonPath =
    process.env.TUTTI_AGENT_SESSION_REPLAY_DAEMON_EXECUTABLE?.trim();
  if (preparedDaemonPath) {
    await access(preparedDaemonPath);
  } else {
    await runCommand("pnpm", ["generate:builtin-apps"], workspaceRoot);
    await buildDaemon(daemonPath);
  }
  return {
    daemonPath: preparedDaemonPath || daemonPath,
    directory,
    stateDirectory,
    userDataDirectory
  };
}

export function managedDesktopLaunch() {
  const command =
    process.env.TUTTI_AGENT_SESSION_REPLAY_ELECTRON_EXECUTABLE?.trim();
  if (!command) {
    throw new Error("managed replay Electron executable is unavailable");
  }
  const entry = process.env.TUTTI_AGENT_SESSION_REPLAY_ELECTRON_ENTRY?.trim();
  return {
    args: entry ? [entry] : [],
    command
  };
}

export function preparedDesktopLaunch() {
  const command =
    process.env.TUTTI_AGENT_SESSION_REPLAY_ELECTRON_EXECUTABLE?.trim();
  if (!command) return undefined;
  const entry = process.env.TUTTI_AGENT_SESSION_REPLAY_ELECTRON_ENTRY?.trim();
  if (!entry) {
    throw new Error("prepared replay Electron entry is unavailable");
  }
  return { args: [entry], command };
}

export async function initializeCleanDatabase(
  workspaceRoot,
  runtime,
  workspaceId,
  { seedWorkspace = true } = {}
) {
  const listenerInfoPath = replayListenerInfoPath(runtime.stateDirectory);
  const daemon = spawn(runtime.daemonPath, [], {
    cwd: workspaceRoot,
    detached: process.platform !== "win32",
    env: {
      ...process.env,
      TUTTI_ANALYTICS_DISABLED: "1",
      TUTTI_ENV: "development",
      TUTTI_STATE_DIR: runtime.stateDirectory,
      TUTTID_ADDR: "127.0.0.1:0",
      TUTTID_ACCESS_TOKEN: randomUUID()
    },
    stdio: ["ignore", "pipe", "pipe"]
  });
  let daemonStdout = "";
  let daemonStderr = "";
  daemon.stdout.on("data", (chunk) => {
    daemonStdout += chunk.toString();
  });
  daemon.stderr.on("data", (chunk) => {
    daemonStderr += chunk.toString();
  });
  try {
    const deadline = Date.now() + 60_000;
    while (Date.now() < deadline) {
      if (daemon.exitCode !== null || daemon.signalCode !== null) {
        throw new Error(
          `tuttid exited while initializing a clean database: ${(
            daemonStderr.trim() ||
            daemonStdout.trim() ||
            daemon.signalCode ||
            daemon.exitCode
          )
            .toString()
            .slice(-4_000)}`
        );
      }
      try {
        await access(listenerInfoPath);
        break;
      } catch {
        await delay(50);
      }
    }
    await access(listenerInfoPath);
  } finally {
    await stopProcessTree(daemon);
  }
  const databasePath = join(runtime.stateDirectory, "tuttid.db");
  const now = Date.now();
  const workbenchSnapshot = replayWorkbenchSnapshot(
    new Date(now).toISOString()
  );
  await runCommand(
    "sqlite3",
    [
      databasePath,
      `
PRAGMA foreign_keys = ON;
${
  seedWorkspace
    ? `INSERT INTO workspaces (
  id, name, created_at_unix_ms, updated_at_unix_ms, last_opened_at_unix_ms
) VALUES (
  '${sqlString(workspaceId)}', 'Replay Scenario', ${now}, ${now}, ${now}
);`
    : ""
}
INSERT INTO desktop_preferences (
  id, locale, theme_source, updated_at_unix_ms
) VALUES (
  'desktop', 'en', 'system', ${now}
)
ON CONFLICT(id) DO NOTHING;
${
  seedWorkspace
    ? `INSERT INTO workspace_workbench_snapshots (
  workspace_id,
  schema_version,
  snapshot_json,
  created_at_unix_ms,
  updated_at_unix_ms
) VALUES (
  '${sqlString(workspaceId)}',
  ${workbenchSnapshot.schemaVersion},
  '${sqlString(JSON.stringify(workbenchSnapshot))}',
  ${now},
  ${now}
);`
    : ""
}
`
    ],
    workspaceRoot
  );
}

export function replayWorkbenchSnapshot(autoOpenedAt) {
  return {
    schemaVersion: 1,
    nodes: [],
    nodeStack: [],
    activeNodeId: null,
    metadata: {
      workspaceOnboarding: {
        autoOpened: true,
        autoOpenedAt,
        schemaVersion: 1
      }
    }
  };
}

export async function enableAgentSessionRecordingFeature(
  databasePath,
  workspaceRoot
) {
  await runCommand(
    "sqlite3",
    [
      databasePath,
      `
UPDATE desktop_preferences
SET feature_flags_json = json_set(
  COALESCE(NULLIF(feature_flags_json, ''), '{}'),
  '$."agent.sessionRecording"',
  json('true')
)
WHERE id = 'desktop';
`
    ],
    workspaceRoot
  );
}

export async function enableAgentSessionRecordingTarget(
  databasePath,
  agentTargetId,
  workspaceRoot
) {
  await runCommand(
    "sqlite3",
    [
      databasePath,
      `
UPDATE agent_targets
SET enabled = 1, updated_at_ms = ${Date.now()}
WHERE id = '${sqlString(agentTargetId)}';
`
    ],
    workspaceRoot
  );
}

export async function setAgentComposerDefaults(
  databasePath,
  agentTargetId,
  { model, permissionModeId, reasoningEffort, speed },
  workspaceRoot
) {
  await runCommand(
    "sqlite3",
    [
      databasePath,
      `
UPDATE desktop_preferences
SET agent_composer_defaults_by_agent_target_json = json_set(
  COALESCE(NULLIF(agent_composer_defaults_by_agent_target_json, ''), '{}'),
  '$."${sqlString(agentTargetId)}".permissionModeId',
  '${sqlString(permissionModeId)}',
  '$."${sqlString(agentTargetId)}".model',
  '${sqlString(model)}',
  '$."${sqlString(agentTargetId)}".reasoningEffort',
  '${sqlString(reasoningEffort)}',
  '$."${sqlString(agentTargetId)}".speed',
  '${sqlString(speed)}'
)
WHERE id = 'desktop';
`
    ],
    workspaceRoot
  );
}

export async function removeRuntime(directory) {
  await rm(directory, {
    recursive: true,
    force: true,
    maxRetries: 10,
    retryDelay: 200
  });
}

async function runCommand(command, args, workspaceRoot) {
  await new Promise((resolveCommand, rejectCommand) => {
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

function sqlString(value) {
  return String(value).replaceAll("'", "''");
}
