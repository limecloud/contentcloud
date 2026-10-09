export function entityDisplayName(value: unknown, fallback: string, reference?: unknown): string {
  const name = typeof value === "string" ? value.trim() : "";
  const opaque = name === reference ||
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(name) ||
    /^[0-9a-f]{12,}$/i.test(name) || /^[0-9A-HJKMNP-TV-Z]{26}$/.test(name) ||
    /^(?:app|org|usr|user|sp|role|mem|mbr|tenant|identity|customer|contact|resource|lead|opp|prod|sub)_(?:[0-9a-f-]{8,}|[0-9A-HJKMNP-TV-Z]{26})$/i.test(name);
  return name && !opaque ? name : fallback;
}
