import { api, post } from './api';
import type { ProviderBinding, ProviderProfile } from './types';

export const SEEDANCE_PROVIDER_ID = 'modelark-seedance25';

export interface ProviderBindingInput {
  profile_version: string;
  state: 'active'|'disabled';
  credential_ref?: string;
  egress_policy: string;
  monthly_budget_minor: number;
  max_job_cost_minor: number;
  max_concurrency: number;
  max_retries: number;
}

const providerPath = (suffix: string) => `/api/bff${suffix}`;

export const providerApi = {
  availableProfiles: (providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderProfile[]>(providerPath(`/provider-profiles?provider_id=${encodeURIComponent(providerID)}`)),
  adminProfiles: (providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderProfile[]>(providerPath(`/admin/provider-profiles?provider_id=${encodeURIComponent(providerID)}`)),
  createProfile: (input: Record<string, unknown>) =>
    post<ProviderProfile>(providerPath('/admin/provider-profiles'), input),
  publishProfile: (providerID: string, version: string) =>
    post<ProviderProfile>(providerPath(`/admin/provider-profiles/${encodeURIComponent(providerID)}/${encodeURIComponent(version)}/publish`)),
  binding: (providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderBinding>(providerPath(`/provider-bindings/${encodeURIComponent(providerID)}`)),
  adminBinding: (tenantID: string, providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderBinding>(providerPath(`/admin/tenants/${encodeURIComponent(tenantID)}/provider-bindings/${encodeURIComponent(providerID)}`)),
  saveBinding: (input: ProviderBindingInput, providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderBinding>(providerPath(`/provider-bindings/${encodeURIComponent(providerID)}`), {
      method: 'PUT',
      body: JSON.stringify(input)
    }),
  saveAdminBinding: (tenantID: string, input: ProviderBindingInput, providerID = SEEDANCE_PROVIDER_ID) =>
    api<ProviderBinding>(providerPath(`/admin/tenants/${encodeURIComponent(tenantID)}/provider-bindings/${encodeURIComponent(providerID)}`), {
      method: 'PUT',
      body: JSON.stringify(input)
    })
};

export function isMissingProviderBinding(value: unknown): boolean {
  return typeof value === 'object' && value !== null && 'status' in value && (value as {status?: number}).status === 404;
}
