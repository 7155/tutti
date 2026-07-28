import assert from "node:assert/strict";
import test from "node:test";
import type {
  AgentActivityAdapter,
  AgentActivityDurableMessage,
  AgentActivityMessage,
  AgentActivityMessagePage
} from "@tutti-os/agent-activity-core";
import { reconcileAgentSessionMessagePages } from "./workspaceAgentActivityReconcileMessages.ts";

test("child cursor advances from assistant-only durable messages", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "child-1",
    cached: [message({ role: "assistant", sequence: 1, version: 7 })],
    cursorPolicy: "durable",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.deepEqual(requests, [
    {
      afterVersion: 7,
      agentSessionId: "child-1",
      order: "asc",
      workspaceId: "workspace-1"
    }
  ]);
});

test("child cursor advances from tool-only durable messages", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "child-1",
    cached: [
      message({
        kind: "tool_call",
        role: "assistant",
        sequence: 4,
        version: 12
      })
    ],
    cursorPolicy: "durable",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.equal(requests[0]?.afterVersion, 12);
  assert.equal(requests[0]?.order, "asc");
});

test("child cursor ignores transient overlays even when their version is higher", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "child-1",
    cached: [
      message({ sequence: 1, version: 5 }),
      message({ messageId: "transient", sequence: undefined, version: 99 })
    ],
    cursorPolicy: "durable",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.equal(requests[0]?.afterVersion, 5);
});

test("root conversation keeps its missing-user history repair policy", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "root-1",
    cached: [message({ role: "assistant", sequence: 1, version: 7 })],
    cursorPolicy: "conversation",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.deepEqual(requests, [
    {
      afterVersion: 0,
      agentSessionId: "root-1",
      order: "asc",
      workspaceId: "workspace-1"
    }
  ]);
});

test("first child read preserves the bounded newest-page contract", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "child-1",
    cached: [],
    cursorPolicy: "durable",
    messageWindowKnown: false,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.deepEqual(requests, [
    {
      agentSessionId: "child-1",
      limit: 100,
      order: "desc",
      workspaceId: "workspace-1"
    }
  ]);
});

test("known-empty child drains every message from authoritative cursor zero", async () => {
  const requests: MessageRequest[] = [];
  const adapter = {
    async listSessionMessages(
      input: MessageRequest
    ): Promise<AgentActivityMessagePage> {
      requests.push({ ...input });
      if (input.afterVersion === 0) {
        return {
          hasMore: true,
          latestVersion: 100,
          messages: Array.from({ length: 100 }, (_, index) =>
            durableMessage({
              messageId: `message-${index + 1}`,
              sequence: index + 1,
              version: index + 1
            })
          )
        };
      }
      assert.equal(input.afterVersion, 100);
      return {
        hasMore: false,
        latestVersion: 200,
        messages: Array.from({ length: 100 }, (_, index) =>
          durableMessage({
            messageId: `message-${index + 101}`,
            sequence: index + 101,
            version: index + 101
          })
        )
      };
    }
  } as AgentActivityAdapter;

  const page = await reconcileAgentSessionMessagePages({
    adapter,
    agentSessionId: "child-1",
    cached: [],
    cursorPolicy: "durable",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.deepEqual(
    requests.map(({ afterVersion, order }) => ({ afterVersion, order })),
    [
      { afterVersion: 0, order: "asc" },
      { afterVersion: 100, order: "asc" }
    ]
  );
  assert.equal(page.messages.length, 200);
  assert.equal(page.latestVersion, 200);
});

test("known-empty root preserves the newest-page repair policy", async () => {
  const requests: MessageRequest[] = [];
  await reconcileAgentSessionMessagePages({
    adapter: adapterRecording(requests),
    agentSessionId: "root-1",
    cached: [],
    cursorPolicy: "conversation",
    messageWindowKnown: true,
    shouldAbort: () => false,
    workspaceId: "workspace-1"
  });

  assert.deepEqual(requests, [
    {
      agentSessionId: "root-1",
      limit: 100,
      order: "desc",
      workspaceId: "workspace-1"
    }
  ]);
});

type MessageRequest = Parameters<
  AgentActivityAdapter["listSessionMessages"]
>[0];

function adapterRecording(requests: MessageRequest[]): AgentActivityAdapter {
  return {
    async listSessionMessages(input): Promise<AgentActivityMessagePage> {
      requests.push({ ...input });
      return {
        hasMore: false,
        latestVersion: input.afterVersion ?? 0,
        messages: []
      };
    }
  } as AgentActivityAdapter;
}

function message(
  overrides: Partial<AgentActivityMessage>
): AgentActivityMessage {
  return {
    ...durableMessage(),
    ...overrides
  } as AgentActivityMessage;
}

function durableMessage(
  overrides: Partial<AgentActivityDurableMessage> = {}
): AgentActivityDurableMessage {
  return {
    agentSessionId: "child-1",
    kind: "message.assistant",
    messageId: "message-1",
    occurredAtUnixMs: 1,
    payload: {},
    role: "assistant",
    sequence: 1,
    turnId: "turn-1",
    version: 1,
    workspaceId: "workspace-1",
    ...overrides
  };
}
