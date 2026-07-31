import assert from "node:assert/strict";
import test from "node:test";
import type {
  AgentActivityCreateSessionInput,
  AgentActivitySendInput,
  AgentPromptContentBlock,
  AgentSessionActivateEffectInput
} from "@tutti-os/agent-activity-core";
import {
  tuttiCreateWorkspaceAgentSessionRequestFromActivation,
  tuttiCreateWorkspaceAgentSessionRequestFromActivity,
  tuttiSendWorkspaceAgentSessionInputRequestFromActivity
} from "./index.ts";

test("create request projection keeps only generated transport fields", () => {
  const input = {
    agentSessionId: "session-1",
    agentTargetId: "target-1",
    browserUse: true,
    capabilityRefs: [{ capability: "tutti", source: "slash_command" }],
    clientSubmitId: "submit-1",
    initialContent: [activityImageBlock()],
    initialTuttiModeActivation: {
      orchestrationIntensity: 70,
      source: "slash_command",
      speed: 40,
      status: "active"
    },
    noProject: true,
    submitDiagnostics: {
      blockCount: 1,
      hasImage: true,
      promptLength: 0,
      queued: false,
      source: "composer",
      submittedAtUnixMs: 10
    },
    visible: true,
    workspaceId: "workspace-1"
  } satisfies AgentActivityCreateSessionInput & { agentSessionId: string };

  const request = tuttiCreateWorkspaceAgentSessionRequestFromActivity(input, {
    recordingId: "recording-1"
  });

  assert.deepEqual(request.initialContent, [
    {
      attachmentId: "attachment-1",
      data: "base64",
      mimeType: "image/png",
      name: "image.png",
      path: "image.png",
      type: "image",
      url: "https://example.test/image.png"
    }
  ]);
  assert.deepEqual(request.capabilityRefs, [
    { capability: "tutti", source: "slash_command" }
  ]);
  assert.deepEqual(request.initialTuttiModeActivation, {
    effect: 70,
    source: "slash_command",
    speed: 40,
    status: "active"
  });
  assert.deepEqual(request.submitDiagnostics, input.submitDiagnostics);
  assert.equal(request.browserUse, true);
  assert.equal(request.noProject, true);
  assert.equal(request.recordingId, "recording-1");
  assert.equal("assetId" in request.initialContent[0]!, false);
  assert.equal("hostPath" in request.initialContent[0]!, false);
  assert.equal("kind" in request.initialContent[0]!, false);
  assert.equal("sizeBytes" in request.initialContent[0]!, false);
  assert.equal("uploadStatus" in request.initialContent[0]!, false);
  assert.equal("uri" in request.initialContent[0]!, false);
});

test("activation request projection shares the generated create contract", () => {
  const input = {
    agentSessionId: "session-1",
    agentTargetId: "target-1",
    clientSubmitId: "submit-1",
    initialContent: [activityImageBlock()],
    mode: "new",
    settings: {
      browserUse: true,
      computerUse: true,
      model: "model-1",
      permissionModeId: "permission-1",
      planMode: true,
      reasoningEffort: "high",
      speed: "fast"
    },
    workspaceId: "workspace-1"
  } satisfies AgentSessionActivateEffectInput;

  const request = tuttiCreateWorkspaceAgentSessionRequestFromActivation(input);

  assert.equal(request.browserUse, true);
  assert.equal(request.model, "model-1");
  assert.equal(request.permissionModeId, "permission-1");
  assert.equal(request.planMode, true);
  assert.equal(request.reasoningEffort, "high");
  assert.equal(request.speed, "fast");
  assert.equal(request.visible, true);
  assert.equal("computerUse" in request, false);
  assert.equal("hostPath" in request.initialContent[0]!, false);
});

test("send request projection allowlists content and omits false guidance", () => {
  const input = {
    agentSessionId: "session-1",
    capabilityRefs: [{ capability: "tutti", source: "slash_command" }],
    clientSubmitId: "submit-1",
    content: [activityImageBlock()],
    displayPrompt: "image",
    guidance: false,
    submitDiagnostics: {
      blockCount: 1,
      submittedAtUnixMs: 10
    },
    workspaceId: "workspace-1"
  } satisfies AgentActivitySendInput;

  const request = tuttiSendWorkspaceAgentSessionInputRequestFromActivity(input);

  assert.equal("guidance" in request, false);
  assert.equal("hostPath" in request.content[0]!, false);
  assert.deepEqual(request.capabilityRefs, input.capabilityRefs);
  assert.deepEqual(request.submitDiagnostics, input.submitDiagnostics);
});

test("request projection rejects local file blocks and unsupported image MIME types", () => {
  const base = {
    agentSessionId: "session-1",
    clientSubmitId: "submit-1",
    displayPrompt: null,
    workspaceId: "workspace-1"
  };
  assert.throws(
    () =>
      tuttiSendWorkspaceAgentSessionInputRequestFromActivity({
        ...base,
        content: [{ hostPath: "/tmp/file.txt", type: "file" }]
      }),
    /File prompt blocks must be uploaded before submission/
  );
  assert.throws(
    () =>
      tuttiSendWorkspaceAgentSessionInputRequestFromActivity({
        ...base,
        content: [{ mimeType: "image/gif", type: "image" }]
      }),
    /Unsupported workspace agent prompt image MIME type/
  );
});

function activityImageBlock(): AgentPromptContentBlock {
  return {
    assetId: "asset-1",
    attachmentId: "attachment-1",
    data: "base64",
    hostPath: "/tmp/image.png",
    kind: "local-image",
    mimeType: "image/png",
    name: "image.png",
    path: "image.png",
    sizeBytes: 42,
    type: "image",
    uploadStatus: "uploaded",
    uri: "file:///tmp/image.png",
    url: "https://example.test/image.png"
  };
}
