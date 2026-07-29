---
name: contentcloud-seedance-export
description: Compile a ContentCloud server-approved and locally digest-verified StoryboardPackage into a deterministic, copy-ready Seedance upload manifest and Chinese prompt package. Use when exporting storyboard frames to Seedance, mapping @图片/@视频/@音频 references, segmenting shot prompts, validating provider limits, or diagnosing a stale package; never upload to Seedance or approve content on the user's behalf.
---

# ContentCloud Seedance Export

Project a locked storyboard into provider-specific operating instructions. Do not change audience strategy, script facts, product claims, storyboard media, or approval state.

## Execution boundary

| Plane | Allowed work |
| --- | --- |
| `Codex local` | Read a pulled storyboard ApprovedSnapshot, verify local digests, compile stable upload numbering and prompts, validate limits/rights/Offer, and write `60-delivery`. |
| `ContentCloud server` | Supply the authoritative storyboard ApprovedSnapshot and optionally store a separately published delivery manifest; it never runs Seedance generation. |
| `User in Seedance` | Log in, inspect disclosure, upload files in order, verify UI reference numbers/settings, paste prompts, start generation, and download takes. |

Do not use a raw local `review_ready` manifest as authority. Require a pulled ApprovedSnapshot whose `submission_type` is `storyboard`, then require the snapshot object's `locked_digest` to match every local input file.

## Workflow

1. Resolve the bound workspace and show the selected storyboard ApprovedSnapshot. Refuse mutable cache files, project/workspace mismatch, missing eligible object IDs, or non-storyboard snapshots.

2. Load the active, human-verified Seedance provider profile. Treat model label, supported modes, file formats, reference counts, duration range, size limits, sound behavior, face policy, and expiry as versioned facts. Do not copy limits from an unfixed upstream `master` branch.

3. Recompute the storyboard manifest and media digests. Stop with `STORYBOARD_LOCKED_DIGEST_MISMATCH` on any drift; never silently regenerate numbering against changed media.

4. Select only model inputs: identity anchors, first/end frames, approved reference video, and approved reference audio. Exclude `review_sheet` unless the verified provider profile explicitly defines a storyboard-board mode.

5. Assign references deterministically: common anchors first, then segment and shot order; deduplicate identical Artifact IDs; number images, videos, and audio independently as `@图片N`, `@视频N`, and `@音频N`.

6. Compile one or more segments along narrative boundaries. Keep one observable action or transition per segment. Preserve outgoing/incoming state between segments. Reject a segment that exceeds the active provider profile instead of mechanically truncating it.

7. Write each Chinese prompt in this order: mode and settings, reference purpose, incoming state, timed observable action, composition/camera/motion, sound intent, outgoing state, product and continuity locks, then negative constraints. Avoid unsupported quality adjectives and conflicting camera instructions.

8. Keep price, coupon, inventory, exact packaging text, subtitles, logo, CTA, legal text, and countdown out of generated plates. Put them in `post_production_plan`, and require a still-valid CommerceOfferSnapshot before final render or Douyin publish when dynamic terms are used.

9. Validate that every `@引用` maps to exactly one upload item, every copied file matches SHA-256, all limits and rights pass, no absolute path or credential is present, and the provider profile has not expired.

10. Run the local exporter with limits read from the selected provider profile, never from guessed defaults:

   ```bash
   contentcloud local seedance export \
     --snapshot <storyboard-approved-snapshot-id> \
     --storyboard <storyboard-package-id> \
     --profile-version <verified-profile-version> \
     --adapter-digest sha256:<adapter-digest> \
     --sound <profile-sound-setting> \
     --min-duration <profile-min> \
     --max-duration <profile-max> \
     --max-images <profile-image-limit> \
     --max-videos <profile-video-limit> \
     --max-audios <profile-audio-limit>
   contentcloud local seedance lint <generated-package.json>
   ```

11. Produce a self-contained directory:

   ```text
   60-delivery/packages/<package-id>/providers/seedance/
     package.json
     README.md
     prompts/segment-01.txt
     media/image-01.<ext>
   ```

12. Present the upload order and prompt files to the user. Stop before opening, uploading, generating, downloading, or publishing unless the user separately performs those external-platform actions.

## Required operator handoff

Make `README.md` sufficient without chat history. Include the locked storyboard snapshot and digest, adapter/profile versions, platform settings, exact upload order and `@引用` mapping, per-segment copy text, expected incoming/outgoing state, acceptance checks, retry scope, and post-production checklist.

After generation, treat downloaded takes as new local candidate artifacts. Human selection, local QA, post-production, final delivery publish, Douyin publish, and server-side creative binding are separate stages with separate authority.
