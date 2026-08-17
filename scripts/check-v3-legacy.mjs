import { readFileSync } from 'node:fs'

const checks = [
  {
    files: [
      'internal/domain/content.go',
      'internal/app/review_export.go',
      'internal/app/lineage.go',
      'internal/store/store.go',
      'internal/store/memory/content.go',
      'internal/store/postgres/review.go',
      'internal/httpapi/content_handlers.go',
      'internal/httpapi/server.go',
      'internal/httpapi/cli_dispatch.go',
      'internal/cli/artifacts.go',
      'internal/cli/root.go',
      'contracts/openapi.yaml',
      'apps/web/src/types.ts',
    ],
    forbidden: [
      'ExtensionArtifactEnvelopeV1',
      'ArtifactPresentation',
      'ArtifactOpenRequest',
      'artifact.register',
      'artifact.open',
      'artifact.presentation',
      'local-open',
      'artifact-open-requests',
    ],
  },
  {
    files: [
      'internal/localworkspace',
      'internal/cli/local_commands.go',
      'internal/cli/submission_commands.go',
      'plugins/contentcloud-video-production/skills',
      'contracts',
      'apps/web/src',
    ],
    forbidden: ['ScriptPackageV2', 'local.script', 'script_files', 'batch.json'],
  },
  {
    files: [
      'internal',
      'contracts',
      'plugins/contentcloud-video-production',
      'apps/web/src',
      'deploy',
    ],
    forbidden: [
      'ScriptVersion',
      'ScriptPackage',
      'ScriptChangeRequest',
      'ScriptCapability',
      'script_generate',
      'script_revise',
      'contentcloud.script.generate',
      'script_versions',
      'script_id',
      'script_refs',
      'DELIVERY_SCRIPT_REQUIRED',
    ],
  },
  {
    files: [
      'internal/domain/content.go',
      'internal/domain/model.go',
      'internal/app',
      'internal/store',
      'internal/httpapi',
      'internal/cli/business_commands.go',
      'internal/cli/root.go',
      'contracts/openapi.yaml',
      'apps/web/src/types.ts',
      'migrations',
    ],
    forbidden: [
      'BenchmarkContent',
      'ContentFramework',
      'ShotPattern',
      'SellingPoint',
      'VisualizationPlan',
      'BriefVersion',
      'brief_versions',
      'benchmark_contents',
      'content_frameworks',
      'shot_patterns',
      'selling_points',
      'visualization_plans',
      'brief.list',
      'brief.show',
      'brief.create',
      'brief.review',
    ],
  },
]

const walkFiles = async (path) => {
  const { readdir } = await import('node:fs/promises')
  const { stat } = await import('node:fs/promises')
  if (!(await stat(path)).isDirectory()) return [path]
  const children = await readdir(path)
  const nested = await Promise.all(children.map((child) => walkFiles(`${path}/${child}`)))
  return nested.flat()
}

const failures = []
const artifactModel = readFileSync('internal/domain/content.go', 'utf8').split('type Artifact struct {', 2)[1]?.split('\n}', 1)[0] ?? ''
if (artifactModel.includes('ScriptVersionID') || artifactModel.includes('approved_snapshot_id,omitempty')) {
  failures.push('internal/domain/content.go: Artifact must require ApprovedSnapshotID and must not expose ScriptVersionID')
}

for (const check of checks) {
  for (const input of check.files) {
    for (const file of await walkFiles(input)) {
      const content = readFileSync(file, 'utf8')
      for (const forbidden of check.forbidden) {
        if (content.includes(forbidden)) failures.push(`${file}: forbidden V3 legacy token ${forbidden}`)
      }
    }
  }
}

if (failures.length > 0) {
  console.error(failures.join('\n'))
  process.exit(1)
}

console.log('V3 legacy runtime guard passed')
