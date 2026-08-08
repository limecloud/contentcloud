import { readFileSync, readdirSync } from 'node:fs'

const failures = []
const read = (path) => readFileSync(path, 'utf8')
const requireText = (body, token, message) => {
  if (!body.includes(token)) failures.push(message)
}
const forbidText = (body, token, message) => {
  if (body.includes(token)) failures.push(message)
}

const batchSchema = JSON.parse(read('contracts/content-batch-3.0.schema.json'))
for (const videoField of ['shots', 'duration_ms', 'aspect_ratio']) {
  if (batchSchema.required?.includes(videoField) || Object.hasOwn(batchSchema.properties ?? {}, videoField)) {
    failures.push(`ContentBatch schema must not own video-only field ${videoField}`)
  }
}
for (const field of ['content_kind', 'content_schema_ref', 'delivery_profiles']) {
  if (!batchSchema.required?.includes(field)) failures.push(`ContentBatch schema must require routing field ${field}`)
}
const contentKinds = batchSchema.properties?.content_kind?.enum ?? []
if (!contentKinds.includes('video_script') || !contentKinds.includes('wechat_article')) {
  failures.push('ContentBatch schema must route both current content kinds')
}

const publicReview = read('web/src/views/PublicViews.tsx')
requireText(publicReview, 'reviewSubject(projection)', 'public review must route through reviewSubject')
requireText(publicReview, '<ReviewContent', 'public review must render the routed review subject')
for (const token of ['.shots', 'duration_ms', 'contentcloud.content-item/3.0']) {
  forbidText(publicReview, token, `public review shell must not assume video field ${token}`)
}
const reviewRouter = read('web/src/views/reviewSubject.ts')
for (const token of ["kind:'video_script'", "kind:'wechat_article'", "contentcloud.content-item/3.0", "contentcloud.article/1.0"]) {
  requireText(reviewRouter, token, `review subject router is missing ${token}`)
}

const publish = read('internal/cli/submission_commands.go')
for (const token of ['case localworkspace.ContentItemSchema:', 'case localworkspace.ArticleSchema:']) {
  requireText(publish, token, `publish validation must explicitly route ${token}`)
}
const mcp = read('internal/cli/workspace_commands.go')
for (const name of ['article_brief_lint', 'article_batch_create', 'article_item_lint', 'article_batch_lint', 'article_batch_finalize', 'article_item_diff', 'wechat_package_export', 'wechat_package_lint']) {
  const marker = `case "${name}":`
  const start = mcp.indexOf(marker)
  const end = mcp.indexOf('\n\tcase "', start + marker.length)
  const branch = start >= 0 ? mcp.slice(start, end >= 0 ? end : undefined) : ''
  if (!branch.includes('requireMCPContentType') || !branch.includes('domain.ContentTypeWeChatArticle')) {
    failures.push(`${name} must verify the signed tenant content capability before acting`)
  }
}
const localCommands = read('internal/cli/local_commands.go')
if ((localCommands.match(/requireLocalContentType\(/g) ?? []).length < 9) {
  failures.push('every local article and WeChat command must verify the tenant content capability')
}

const skillRoot = 'plugins/contentcloud-wechat-article/skills'
const expectedSkills = ['contentcloud-article-planning', 'contentcloud-article-visuals', 'contentcloud-longform-writing', 'contentcloud-wechat-delivery']
const actualSkills = readdirSync(skillRoot, { withFileTypes: true }).filter((entry) => entry.isDirectory()).map((entry) => entry.name).sort()
if (JSON.stringify(actualSkills) !== JSON.stringify(expectedSkills)) {
  failures.push(`WeChat Skill Pack contents ${JSON.stringify(actualSkills)} do not match ${JSON.stringify(expectedSkills)}`)
}
for (const skill of expectedSkills) {
  const body = read(`${skillRoot}/${skill}/SKILL.md`)
  for (const token of ['workspace_context', 'wechat_article']) requireText(body, token, `${skill} must require ${token}`)
  for (const token of ['UpdatePlatformTenantContentCapability', 'setTenantContentCapability', '/content-capabilities/']) {
    forbidText(body, token, `${skill} must not own tenant capability mutation through ${token}`)
  }
}
const deliverySkill = read(`${skillRoot}/contentcloud-wechat-delivery/SKILL.md`)
for (const token of ['manual_login', 'manual_asset_upload', 'manual_preview', 'manual_publish', 'never logs in to WeChat']) {
  requireText(deliverySkill, token, `WeChat delivery Skill is missing manual boundary ${token}`)
}

const plugin = JSON.parse(read('plugins/contentcloud-wechat-article/plugin.json'))
if (plugin.name !== 'contentcloud-wechat-article' || plugin.extensions?.['run.zhongcao.contentcloud']?.claims !== './run.zhongcao.contentcloud/claims.json') {
  failures.push('WeChat Skill Pack must expose its governed run claims')
}

const tenantDomain = read('internal/domain/platform.go')
requireText(tenantDomain, 'DefaultProjectContentType = ContentTypeMarketingVideo', 'marketing_video must remain the default Project content type')
requireText(tenantDomain, 'result := []string{ContentTypeVideoScript}', 'video_script must remain the always-enabled baseline content type')
if (!/ContentTypeMarketingVideo:\s*\{\}/.test(tenantDomain)) failures.push('marketing_video must remain an optional tenant capability')
if (!/ContentTypeWeChatArticle:\s*\{\}/.test(tenantDomain)) failures.push('wechat_article must remain an optional tenant capability')
for (const file of ['internal/cli/local_commands.go', 'internal/cli/workspace_commands.go', ...expectedSkills.map((skill) => `${skillRoot}/${skill}/SKILL.md`)]) {
  forbidText(read(file), 'UpdatePlatformTenantContentCapability', `${file} must not enable tenant content capabilities`)
}

const workspaceRouter = read('web/src/router.tsx')
if ((workspaceRouter.match(/web\/src\/workspace|\.TaskProductionPage/g) ?? []).length !== 0) {
  failures.push('retired workspace task routes must not be reintroduced')
}
forbidText(workspaceRouter, 'WorkOSTaskDetailPage', 'web/src/router.tsx must not restore the retired task detail page')

if (failures.length > 0) {
  console.error(failures.join('\n'))
  process.exit(1)
}

console.log('Multi-content governance guard passed')
