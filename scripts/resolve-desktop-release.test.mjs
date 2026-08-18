import assert from "node:assert/strict";
import { test } from "node:test";

import { resolveDesktopReleaseIdentity } from "./resolve-desktop-release.mjs";

test("resolves a stable tag push", () => {
  assert.deepEqual(
    resolveDesktopReleaseIdentity({
      eventName: "push",
      eventTag: "v0.28.0",
      githubSha: "release-commit",
    }),
    {
      tag: "v0.28.0",
      version: "0.28.0",
      channel: "stable",
      sourceRef: "release-commit",
      publish: true,
    },
  );
});

test("resolves a prerelease tag push into beta", () => {
  const identity = resolveDesktopReleaseIdentity({
    eventName: "push",
    eventTag: "v0.29.0-beta.1",
    githubSha: "prerelease-commit",
  });
  assert.equal(identity.channel, "beta");
  assert.equal(identity.publish, true);
});

test("defaults a manual build to its immutable version tag", () => {
  const identity = resolveDesktopReleaseIdentity({
    eventName: "workflow_dispatch",
    inputVersion: "0.28.0",
    inputPublish: false,
  });
  assert.equal(identity.tag, "v0.28.0");
  assert.equal(identity.sourceRef, "v0.28.0");
  assert.equal(identity.channel, "stable");
});

test("allows a custom source ref only for unpublished artifacts", () => {
  const identity = resolveDesktopReleaseIdentity({
    eventName: "workflow_dispatch",
    inputVersion: "v0.28.0",
    inputSourceRef: "feature/desktop-preview",
    inputPublish: false,
  });
  assert.equal(identity.sourceRef, "feature/desktop-preview");
});

test("rejects a custom source ref for a published release", () => {
  assert.throws(
    () =>
      resolveDesktopReleaseIdentity({
        eventName: "workflow_dispatch",
        inputVersion: "v0.28.0",
        inputSourceRef: "main",
        inputPublish: true,
      }),
    /must build from v0\.28\.0, not main/,
  );
});

test("accepts the exact tag for a manual published release", () => {
  const identity = resolveDesktopReleaseIdentity({
    eventName: "workflow_dispatch",
    inputVersion: "v0.28.0",
    inputSourceRef: "v0.28.0",
    inputPublish: true,
  });
  assert.equal(identity.sourceRef, "v0.28.0");
  assert.equal(identity.publish, true);
});

test("rejects the retired desktop-only tag", () => {
  assert.throws(
    () =>
      resolveDesktopReleaseIdentity({
        eventName: "workflow_dispatch",
        inputVersion: "desktop-v0.28.0",
      }),
    /invalid desktop release tag/,
  );
});

test("rejects a stable version published to beta", () => {
  assert.throws(
    () =>
      resolveDesktopReleaseIdentity({
        eventName: "workflow_dispatch",
        inputVersion: "v0.28.0",
        inputChannel: "beta",
        inputPublish: true,
      }),
    /published beta requires a prerelease version/,
  );
});
