---
name: contentcloud-storyboard-production
description: Build, generate, validate, publish, and revise local storyboard image packages from an approved ContentItem in a bound ContentCloud workspace. Use for ContentItem-to-shot planning, first/end-frame production, review-sheet preparation, storyboard review handoff, or digest drift diagnosis; enforce that Codex produces candidates while ContentCloud server approval creates the only authoritative locked snapshot.
---

# ContentCloud Storyboard Production

Produce independently reviewable first/end frames from an approved provider-neutral ContentItem. Preserve product truth, continuity, rights, citations, and experiment intent.

## Execution boundary

| Plane | Allowed work |
| --- | --- |
| `Codex local` | Pull approved content, create shot tasks, call an authorized image capability, write local media, calculate digests, generate a review sheet, and lint a `review_ready` candidate. |
| `ContentCloud server` | Receive an explicit storyboard publish, validate the submitted revision, host review, create the ApprovedSnapshot, and record the lock decision and audit trail. |
| `Human` | Select generated frames, judge product truth and continuity, authorize disclosure, confirm publish, and approve or request changes. |

`StoryboardPackage.status=review_ready` is local readiness only. It is not `approved` or `locked`. A storyboard is locked only when a `storyboard` ApprovedSnapshot has been pulled from ContentCloud and the packaged `locked_digest` still matches local media.

## Workflow

1. Require a pulled `content_batch` ApprovedSnapshot containing a deliverable ContentItem. Never start from an unapproved `50-production/batches` candidate.

2. Create the local storyboard package:

   ```bash
   contentcloud local storyboard create \
     --snapshot <content-approved-snapshot-id> \
     --content-item <content-item-id> \
     --capability-id <image-capability-id> \
     --capability-version <version> \
     --capability-digest sha256:<digest>
   ```

3. For each generated shot directory, write exactly one `first-frame` image and an optional `end-frame` image. Keep the file extension supported by the active capability. Generate `review-sheet` at the package root for human review.

4. Use approved real product assets whenever appearance matters. Do not regenerate SKU shape, packaging text, ports, accessories, scale, certification, price, discount, or product result. Switch to the declared Plan B when product truth cannot be preserved.

5. Preserve every shot's incoming/outgoing state, movement axis, lighting lock, product lock, anchors, rights, knowledge, claim references, negative constraints, and acceptance criteria. A review sheet is never a default video-model reference.

6. Discover media and prepare review:

   ```bash
   contentcloud local storyboard prepare <manifest.json>
   contentcloud local storyboard lint <manifest.json>
   ```

7. Run storyboard publish preflight. Confirm the exact disclosure list and plan before sending anything to the server:

   ```bash
   contentcloud publish storyboard --file <manifest.json> --dry-run
   ```

8. Stop after publish. The user or authorized reviewer completes review on ContentCloud server. Do not mutate the local manifest to `approved` or `locked`, and do not create a fake ApprovedSnapshot.

9. After server approval, pull the exact snapshot:

   ```bash
   contentcloud pull approved --type storyboard
   ```

10. Before handing off to Seedance export, verify all local file SHA-256 values and the package `locked_digest` against the pulled snapshot. Any changed, replaced, recompressed, cropped, or renamed file requires a new local candidate and a new review revision.

## Review requirements

Require human review of narrative alignment, audience strategy, product appearance and usage, first/end-state continuity, movement axis, lighting, identity anchors, rights, 9:16 safe composition, subtitle space, and observable acceptance criteria.

Stop if any first frame, review sheet, right, Plan B, capability digest, or approved upstream reference is missing. Report the exact shot and next local or server action without crossing the execution boundary.
