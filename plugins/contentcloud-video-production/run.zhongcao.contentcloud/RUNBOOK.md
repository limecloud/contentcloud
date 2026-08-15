# ContentCloud Video Production Support

## Health checks

1. Load the package with the ContentCloud Agent Plugins loader.
2. Confirm all seven skills are valid and `contentcloud-local` is available over stdio.
3. Confirm the installed receipt matches the package version and digest.
4. Start a new host session after install, update, repair, or removal.

## Recovery

- A digest mismatch requires a fresh immutable package; do not repair files in place.
- A failed install must leave the previous active release unchanged.
- An MCP startup failure affects only `contentcloud-local`; keep valid skills discoverable and report the component diagnostic.
- A revoked release cannot be installed or used for a new task.

## Seedance 2.5 execution

- `contentcloud-seedance-execution` only operates the ContentCloud Media Job control plane. Do not add ModelArk or `modelark-mcp` to this plugin's `mcp.json`.
- Provider credentials stay in the worker deployment SecretRef/environment. They must not enter Skills, Job events, prompts, or Artifact metadata.
- A Media Job must reference an approved StoryboardSnapshot and PromptPackageArtifact. Local paths and unbounded URLs are invalid inputs.
- The first release supports one segment with `text_to_video` or `image_to_video`; `extend`, editing, audio-driven generation, and multi-segment fan-out stay disabled.
- A provider timeout or unknown cancellation is reconciled before any retry. Downloaded output is a candidate Artifact until technical and content review complete.

## Escalation evidence

Provide the plugin ID, version, package digest, host ID, plan digest, receipt status, stable error code, and redacted diagnostics. Do not include credentials, environment values, cookies, or absolute user paths.
