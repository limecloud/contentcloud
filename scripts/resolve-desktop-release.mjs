import { appendFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const releaseTagPattern =
  /^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$/;

function parsePublish(value) {
  if (value === true || value === "true") return true;
  if (value === false || value === "false" || value === "") return false;
  throw new Error("publish must be true or false");
}

export function resolveDesktopReleaseIdentity({
  eventName,
  eventTag = "",
  githubSha = "",
  inputVersion = "",
  inputChannel = "stable",
  inputSourceRef = "",
  inputPublish = false,
}) {
  if (!["push", "workflow_dispatch"].includes(eventName)) {
    throw new Error(`unsupported desktop release event: ${eventName}`);
  }

  const publish = eventName === "push" ? true : parsePublish(inputPublish);
  const rawVersion =
    eventName === "push"
      ? eventTag.trim()
      : inputVersion.trim().replace(/^v/, "");
  const tag = eventName === "push" ? rawVersion : `v${rawVersion}`;
  if (!releaseTagPattern.test(tag)) {
    throw new Error(`invalid desktop release tag: ${tag}`);
  }

  const version = tag.slice(1);
  const prerelease = version.includes("-");
  const channel = prerelease
    ? "beta"
    : eventName === "push"
      ? "stable"
      : inputChannel || "stable";
  if (!["stable", "beta"].includes(channel)) {
    throw new Error(`invalid desktop release channel: ${channel}`);
  }
  if (channel === "stable" && prerelease) {
    throw new Error("a prerelease version cannot enter stable");
  }
  if (publish && channel === "beta" && !prerelease) {
    throw new Error("a published beta requires a prerelease version");
  }

  let sourceRef;
  if (eventName === "push") {
    sourceRef = githubSha.trim();
    if (!sourceRef) throw new Error("tag pushes require GITHUB_SHA");
  } else {
    const requestedSourceRef = inputSourceRef.trim();
    if (publish && requestedSourceRef && requestedSourceRef !== tag) {
      throw new Error(
        `published desktop releases must build from ${tag}, not ${requestedSourceRef}`,
      );
    }
    sourceRef = publish ? tag : requestedSourceRef || tag;
  }

  return { tag, version, channel, sourceRef, publish };
}

async function main() {
  const identity = resolveDesktopReleaseIdentity({
    eventName: process.env.EVENT_NAME,
    eventTag: process.env.EVENT_TAG,
    githubSha: process.env.GITHUB_SHA,
    inputVersion: process.env.INPUT_VERSION,
    inputChannel: process.env.INPUT_CHANNEL,
    inputSourceRef: process.env.INPUT_SOURCE_REF,
    inputPublish: process.env.INPUT_PUBLISH,
  });
  const output = [
    `tag=${identity.tag}`,
    `version=${identity.version}`,
    `channel=${identity.channel}`,
    `source_ref=${identity.sourceRef}`,
    `publish=${identity.publish}`,
  ].join("\n");
  if (process.env.GITHUB_OUTPUT) {
    await appendFile(process.env.GITHUB_OUTPUT, `${output}\n`, "utf8");
  } else {
    console.log(output);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main().catch((error) => {
    console.error(`desktop release identity failed: ${error.message}`);
    process.exitCode = 1;
  });
}
