# Script Package

The runtime-provided schema remains authoritative. Local CreativeBatch work uses `contentcloud.script-package/2.0`; Automation may declare the compatible `script-package/1.1` contract.

## Top-level intent

- `deliverability`: `review_ready` only when every blocking rule passes; otherwise `blocked`.
- `project_id`, `creative_batch_id`, `brief_version_id`, and `context_snapshot_id`: frozen local lineage.
- `direction`: the selected angle, hook, motif, narrative, tone, emotion, and risks.
- `cover`: first-view product or brand signal, visual intent, assets, rights, safe area, and occlusion guards.
- `narrative_structure`: ordered decision functions mapped to time ranges and shot IDs.
- `shots`: complete, continuous timeline.
- `citations`: explicit mapping from a knowledge ID to a shot and usage.
- `asset_requirements`: truth level, rights, purpose, and fallback.
- `experiment`: one primary variable, controlled dimensions, hypothesis, measurement window, and metrics.
- `global_constraints`: forbidden claims, brand rules, product truth, continuity, and safe areas.

## Shot contract

For each shot provide:

- Stable `shot_id`, continuous `start_ms`/`end_ms`, and one narrative `role`.
- Decision-oriented `narrative_purpose` and observable `visual_intent`.
- Subject, physical action, composition, camera motion, sound, optional voiceover and on-screen text.
- `first_frame`, `motion_spec`, and `end_frame` as three compatible states.
- Eligible `knowledge_refs`, approved claims, assets, and valid rights only.
- One production mode: `real_asset`, `asset_guided_generation`, `generated_non_product`, `composite`, or `external_capture`.
- Negative constraints, continuity in/out plus anchors, product-truth strategy, measurable acceptance criteria, and a practical Plan B.

Required local review-ready roles are `hook`, `proof`, `cta`, and one of `product_intro|product_solution`. Shot timecodes must start at zero, remain contiguous, and end at `duration_ms`.

## Product truth strategies

- `real_asset_composite`: use real packaging, logo, label, certification, or readable product material.
- `generated_environment`: generate only the surrounding environment; protect product truth with real assets.
- `no_product_detail`: keep generated product-like forms generic and never imply they are an exact product representation.

## Citation usage

Use `spoken_claim`, `on_screen_text`, `visual_fact`, or `style_rule`. A citation may reference only an eligible knowledge ID in the frozen local context or Automation contract.
