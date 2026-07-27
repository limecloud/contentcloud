# Environment Lifecycle

## Inspect

1. Resolve one bound workspace.
2. Read the Environment Lock and offline doctor report.
3. Compare installed Plugin, CLI, MCP, Skill, Schema, and routing digests with the signed desired state.
4. Classify the environment as `ready`, `update_available`, `repair_required`, or `blocked`.

## Change

1. Produce a dry-run plan without consuming a connect key or changing files.
2. Show capability, version, permission, network, file, provider, and cost changes.
3. Require explicit confirmation.
4. Back up only the ContentCloud-owned configuration targets.
5. Apply fixed versions and verified digests from the allowlist.
6. Validate the resulting files and run offline doctor.
7. Restore the previous target when validation fails.
8. Report each target independently.

Never change another Marketplace, plugin, MCP server, Skill, user instruction, or unowned configuration block.

## Reconnect

Plugin, Skill, MCP, or project-routing changes require a new Codex chat or CLI session. Create a bootstrap handoff without secrets and open the verified Workspace Root. If opening fails, return the local path and recovery prompt.

## Upgrade and Reset

Do not upgrade during an active interactive or Automation Run. A reset restores only ContentCloud-managed environment files and never removes source material, knowledge, briefs, scripts, media, submissions, approvals, or unrelated Agent configuration.
