---
name: contentcloud-longform-writing
description: Generate or revise cited ContentCloud ArticleItem candidates inside a governed WeChat article ContentBatch. Use for long-form drafting, title variants, structured article blocks, assertions, editorial review, controlled revisions, or publishing a reviewable WeChat article revision from an approved ArticleBrief.
---

# ContentCloud Longform Writing

Write provider-neutral `contentcloud.article/1.0` objects from immutable ContentCloud inputs. Canonical article content is structured data, not arbitrary HTML.

## Preconditions

1. Call `workspace_context`; require a verified `wechat_article` capability, matching environment lock, selected LocalRun, and active claim.
2. Use `article_batch_create` to freeze one approved ArticleBrief and its eligible Knowledge ApprovedSnapshot.
3. Read only the returned `manifest.yaml` and `context.json` under the new batch directory. Do not reconstruct frozen context from chat history or newer workspace files.
4. Write candidates only to the item paths inside that batch.

## Draft

1. Produce several title candidates with explicit strategies and risk refs, then select exactly one title ID.
2. Build the article from supported semantic blocks: `heading`, `paragraph`, `list`, `quote`, `image`, `callout`, `divider`, and `cta`.
3. Keep one editorial purpose per block. Use stable block and assertion IDs so review comments and revisions remain addressable.
4. Classify assertions as `fact`, `commercial_claim`, `quotation`, `editorial_opinion`, `personal_experience`, or `hypothesis`.
5. Cite every fact, commercial claim, and quotation. A `commercial_claim` may reference only an approved Knowledge item whose kind is `claim`; preserve attribution for quotations.
6. Keep editorial opinion, personal experience, and hypotheses visibly distinct from verified fact. Never fabricate experience or attribution.
7. Match the ArticleBrief word range, voice, structure, CTA, approved claims, and controlled variables. Use `blocked` with actionable reasons when a required gate fails.

Do not embed HTML, scripts, tracking, remote media, credentials, or provider-specific draft IDs in an ArticleItem.

## Validate And Review

1. Call `article_item_lint` for every candidate.
2. Call `article_batch_lint` with the exact manifest and complete item list.
3. Call `article_batch_finalize` only after deterministic checks pass. A blocked batch may be reviewed but is not deliverable.
4. Publish the exact `content_batch` files only through `publish_preflight`, explicit confirmation of its `plan_id`, and `publish_apply`.

For a revision, set `based_on_version_id`, resolved comment IDs, and `change_summary`. Call `article_item_diff` with explicit allowed JSON Pointer prefixes. Treat any undeclared path change as an error, then repeat item and batch lint before publishing.
