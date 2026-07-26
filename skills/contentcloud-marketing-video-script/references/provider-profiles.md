# Provider Export Profiles

Last reviewed: 2026-07-25. Treat every value as export guidance that must be revalidated against the actual product surface.

## Generic domestic-tool profile

- Prefer concise Chinese prompts with separate subject, action, camera, sound, and constraints.
- Keep each generated segment short enough to retry independently.
- Reference exact packaging and readable text through real assets and later compositing.
- Keep negative constraints in a separate derived field only when the selected tool exposes one.

## Seedance-family profile

- Multi-reference and multi-shot features vary by Doubao, Jimeng, Dreamina, and API surfaces.
- Validate current duration, prompt length, reference count, and negative-prompt support at export time.
- Avoid relying on generated product text or named intellectual property.

## Kling-family profile

- Validate current duration and multi-shot availability at export time.
- Keep pre-generation safety wording concise; one rejected term can invalidate an entire submission on some surfaces.

The canonical Script Package must not contain hard-coded provider quotas. A versioned exporter converts the package into the chosen profile and records the profile version.
