# ContentCloud Video Production Support

## Health checks

1. Load the package with the ContentCloud Agent Plugins loader.
2. Confirm all six skills are valid and `contentcloud-local` is available over stdio.
3. Confirm the installed receipt matches the package version and digest.
4. Start a new host session after install, update, repair, or removal.

## Recovery

- A digest mismatch requires a fresh immutable package; do not repair files in place.
- A failed install must leave the previous active release unchanged.
- An MCP startup failure affects only `contentcloud-local`; keep valid skills discoverable and report the component diagnostic.
- A revoked release cannot be installed or used for a new task.

## Escalation evidence

Provide the plugin ID, version, package digest, host ID, plan digest, receipt status, stable error code, and redacted diagnostics. Do not include credentials, environment values, cookies, or absolute user paths.
