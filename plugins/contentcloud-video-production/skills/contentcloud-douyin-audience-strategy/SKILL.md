---
name: contentcloud-douyin-audience-strategy
description: Generate, compare, validate, and publish evidence-gated Douyin commerce audience strategy candidates in a bound ContentCloud workspace. Use for single-audience strategy, 2-3 audience comparison, eight-audience exploration, audience-to-Brief handoff, or revising an AudienceStrategyVersion; keep Codex local generation separate from ContentCloud server approval.
---

# ContentCloud Douyin Audience Strategy

Turn a server-governed audience taxonomy and approved project evidence into local strategy candidates. Never treat a local file or model recommendation as an approved strategy.

## Execution boundary

Use exactly these planes:

| Plane | Allowed work |
| --- | --- |
| `Codex local` | Read pulled snapshots, scaffold candidates, compare audiences, edit local JSON, run lint, and prepare publish preflight. |
| `ContentCloud server` | Store taxonomy governance facts, create immutable SubmissionRevision, run review, create ApprovedSnapshot, and record audit history. |
| `Human` | Select audiences, verify evidence, confirm publish, and approve or request changes on the server. |

Do not create an `approved` object locally. `publish` creates a reviewable revision, not an approval. Only a snapshot returned by `contentcloud pull approved` is authoritative.

## Workflow

1. Inspect the bound workspace and current approved inputs. Pull the current strategy snapshots when the user explicitly asks to refresh:

   ```bash
   contentcloud pull approved --type strategy
   ```

2. Require a non-expired, human-verified `AudienceTaxonomySnapshot` from the pulled immutable cache. Do not infer or silently update the eight-audience taxonomy from general model knowledge.

3. Choose one mode:

   - `single`: require exactly one audience code.
   - `compare`: require two or three audience codes and one shared objective.
   - `explore`: create eight lightweight strategy cards only; do not generate eight scripts, storyboards, images, or videos.

4. Scaffold local candidates:

   ```bash
   contentcloud local audience strategy scaffold \
     --taxonomy <taxonomy-object-id> \
     --mode <single|compare|explore> \
     --audience <code> \
     --objective <objective>
   ```

5. Fill each candidate with demand moment, evidence-bounded insight, hook hypotheses, proof order, objections, CTA strategy, evidence references, experiment type, primary variable, controlled variables, target metrics, and constraints.

6. Separate evidence from hypotheses. Keep model-only claims at `candidate` with low confidence. Do not infer sensitive attributes, income, family structure, health status, or purchasing power from an audience label.

7. Validate every selected candidate:

   ```bash
   contentcloud local audience strategy lint <strategy.json>
   ```

8. Run `contentcloud publish strategy --file <strategy.json> --dry-run`. Show the exact preflight and wait for explicit confirmation of its `plan_id` before the cloud write. Publishing crosses from `Codex local` to `ContentCloud server`.

9. Stop after publish unless the user explicitly asks to perform a server review action and has authority to do so. Never approve on the user's behalf.

10. After human approval, run `contentcloud pull approved --type strategy`. Use only the pulled ApprovedSnapshot when producing the Brief or ContentBatch.

## Experiment rules

- Use `strict_ab` only when audience is the sole primary variable and creative, Offer, budget logic, timing, landing page, and observation window remain controlled.
- Use `audience_expression_fit_test` when both audience and expression are intentionally paired.
- Use `exploration_batch` for broad discovery. Do not report it as a causal audience test.
- Require the chosen strategy to state the decision metric and measurement window before Brief creation.

## Stop conditions

Stop with a structured blocker when the taxonomy is absent or expired, evidence references are missing, the Offer is invalid, a strategy contains unsupported product claims, the experiment type conflicts with changed variables, or the workspace has not pulled the required ApprovedSnapshot.

Report the failing gate, the local file involved, and the next valid command. Do not repair formal facts by editing pulled cache files.
