# Script Package 1.1

Produce the schema identified by `script-package/1.1`. The runtime-provided `output.schema.json` remains authoritative.

## Top-level intent

- `deliverability`: `review_ready` only when every blocking rule passes; otherwise `blocked`.
- `creative_strategy`: objective, audience, demand moment, selling points, CTA, hypothesis, single test variable, and invariant fields.
- `production_bible`: reusable subject identity anchors, wardrobe and props, scene lock, visual-style lock, and allowed asset IDs.
- `narrative`: ordered functions used by the shot list.
- `shots`: complete, continuous timeline.
- `citations`: explicit mapping from a knowledge ID to a shot and usage.

## Shot contract

For each shot provide:

- Stable `shot_id`, continuous `start_ms`/`end_ms`, and one narrative `role`.
- Decision-oriented `narrative_purpose` and observable `visual_intent`.
- Subject, physical action, composition, camera motion, sound, optional voiceover and on-screen text.
- `first_frame`, `motion_spec`, and `end_frame` as three compatible states.
- Approved `knowledge_refs` and allowed `reference_asset_ids` only.
- Negative constraints, continuity in/out, product-truth strategy, measurable acceptance criteria, and a practical Plan B for high-risk shots.

Required narrative roles for review-ready output are `hook`, `product_solution`, `proof`, and `cta`. Shot timecodes must start at zero, remain contiguous, and end at `target_duration_seconds * 1000`.

## Product truth strategies

- `real_asset_composite`: use real packaging, logo, label, certification, or readable product material.
- `generated_environment`: generate only the surrounding environment; protect product truth with real assets.
- `no_product_detail`: keep generated product-like forms generic and never imply they are an exact product representation.

## Citation usage

Use `spoken_claim`, `on_screen_text`, `visual_fact`, or `style_rule`. A citation may reference only an approved knowledge ID delivered in the Task Contract.
