import assert from 'node:assert/strict';
import { generateKeyPairSync } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  publicKeyFingerprint,
  registrySignaturePayloadDigest,
  signRegistryEntry,
  verifyRegistryEntrySignature,
} from './plugin-release.mjs';

const entry = {
  id: 'contentcloud-video-production',
  kind: 'scene_plugin',
  version: '0.4.0',
  source: { repository: 'https://github.com/limecloud/contentcloud', ref: 'v0.4.0' },
  license: 'Apache-2.0',
  digest: `sha256:${'a'.repeat(64)}`,
  signature: { status: 'pending' },
  compatible_profiles: ['contentcloud.video-production'],
  permissions: ['workspace:read'],
  data_flow: { local_by_default: true, cloud_actions: [] },
  cost: { model: 'included', notice: 'Included with the ContentCloud subscription.' },
  output_schemas: ['contracts/script-package-2.0.schema.json'],
  evaluation: {
    status: 'passed',
    report: '.agents/plugins/evaluations/contentcloud-video-production-0.4.0.json',
    digest: `sha256:${'b'.repeat(64)}`,
    evidence: ['script-contract'],
  },
  lifecycle: 'evaluated',
  revocation: { status: 'active' },
};

test('Ed25519 registry signature verifies only the canonical release payload', () => {
  const { privateKey, publicKey } = generateKeyPairSync('ed25519');
  const trustStore = {
    keys: [{
      key_id: 'release-2026',
      algorithm: 'ed25519',
      status: 'active',
      public_key: publicKey.export({ type: 'spki', format: 'pem' }),
    }],
  };
  const signature = signRegistryEntry(entry, privateKey.export({ type: 'pkcs8', format: 'pem' }), 'release-2026');
  const signedEntry = { ...entry, signature };

  assert.deepEqual(verifyRegistryEntrySignature(signedEntry, trustStore), {
    key_id: 'release-2026',
    payload_digest: registrySignaturePayloadDigest(entry),
  });
  assert.throws(
    () => verifyRegistryEntrySignature({ ...signedEntry, digest: `sha256:${'c'.repeat(64)}` }, trustStore),
    /verification failed/,
  );
  assert.throws(
    () => verifyRegistryEntrySignature(signedEntry, { keys: [{ ...trustStore.keys[0], status: 'revoked' }] }),
    /not active/,
  );
});

test('lifecycle and revocation changes invalidate the release payload', () => {
  assert.notEqual(
    registrySignaturePayloadDigest(entry),
    registrySignaturePayloadDigest({ ...entry, lifecycle: 'published' }),
  );
  assert.notEqual(
    registrySignaturePayloadDigest(entry),
    registrySignaturePayloadDigest({ ...entry, lifecycle: 'revoked', revocation: { status: 'revoked', severity: 'high', reason: 'incident' } }),
  );
});

test('public key fingerprint accepts PEM and a public KeyObject', () => {
  const { publicKey } = generateKeyPairSync('ed25519');
  assert.equal(
    publicKeyFingerprint(publicKey),
    publicKeyFingerprint(publicKey.export({ type: 'spki', format: 'pem' })),
  );
});

test('Node canonical payload matches the shared Go conformance vector', () => {
  const fixturePath = resolve(fileURLToPath(new URL('../..', import.meta.url)), 'contracts/plugin-release-signature-v1.fixture.json');
  const fixture = JSON.parse(readFileSync(fixturePath, 'utf8'));
  assert.equal(registrySignaturePayloadDigest(fixture.entry), fixture.payload_sha256);
});
