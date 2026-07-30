---
name: contentcloud-wechat-delivery
description: Export and validate an operator-ready local WeChat Official Account package from an approved ContentCloud ArticleItem. Use for safe semantic HTML, Markdown, JSON, asset mappings, local preview, operator instructions, or manual handoff after the exact ArticleItem ApprovedSnapshot has been pulled.
---

# ContentCloud WeChat Delivery

Compile a verified ApprovedSnapshot into local delivery files. The first release is manual delivery only: it never logs in to WeChat, creates a platform draft, uploads assets, or publishes externally.

## Preconditions

1. Call `workspace_context`; require `wechat_article` in the signed Manifest and a matching environment lock.
2. Require an explicitly pulled, digest-verified `content_batch` ApprovedSnapshot containing the requested `review_ready` ArticleItem.
3. Stop if the approved object, content digest, asset rights, or channel profile is missing or stale. Never export an unpublished local candidate.

## Export And Verify

1. Call `wechat_package_export` with the exact approved ArticleItem ID. Keep output under `60-delivery/packages/`.
2. Call `wechat_package_lint` on the returned `providers/wechat-official-account/package.json`.
3. Verify the package contains governed JSON, Markdown, safe semantic HTML, operator instructions, local preview, file digests, asset mappings, and the source ApprovedSnapshot ID.
4. Open only the generated local preview when the user asks to inspect it. Treat the preview as derived output, not canonical content.
5. Report unresolved `manual_asset_upload` mappings before handoff.

## Manual Handoff

Tell the operator to follow the generated `README.md` and perform these external actions manually:

```text
manual_login
manual_asset_upload
manual_preview
manual_publish
record_external_binding
```

Do not automate those steps, request account credentials, claim a draft was created, or claim publication succeeded. Package generation and package lint have no external WeChat side effect. Record an external binding or result only through a separately authorized ContentCloud workflow after the operator confirms what happened.
