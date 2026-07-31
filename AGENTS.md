# Codex Project Instructions

## Design System

Always read `DESIGN.md` before making visual or UI decisions. Font choices, colors, spacing, iconography, and aesthetic direction are defined there. Do not deviate without explicit approval. During UI review, flag code that does not match `DESIGN.md`.

## Skill Routing

When the user's request matches an available skill, use that skill. When in doubt, inspect the relevant skill before changing the project.

- Product ideas and brainstorming: `office-hours`
- Strategy and scope: `plan-ceo-review`
- Architecture: `plan-eng-review`
- Design systems and design plans: `design-consultation` or `plan-design-review`
- Full review pipeline: `autoplan`
- Bugs and errors: `investigate`
- Application QA: `qa` or `qa-only`
- Code and diff review: `review`
- Visual polish: `design-review`
- Shipping and deployment: `ship` or `land-and-deploy`
- Release documentation: `document-release`

Do not run `git commit`, `git push`, branch operations, destructive cleanup, production API calls, or deployment unless the user explicitly requests and confirms the action.
