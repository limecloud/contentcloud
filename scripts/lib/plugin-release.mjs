import { createHash, createPrivateKey, createPublicKey, sign, verify } from 'node:crypto';

const signatureContext = 'contentcloud.plugin-release-signature.v1';
const signedFields = [
  'id',
  'kind',
  'version',
  'source',
  'license',
  'digest',
  'compatible_profiles',
  'permissions',
  'data_flow',
  'cost',
  'output_schemas',
  'evaluation',
  'lifecycle',
  'revocation',
];

export function registrySignaturePayload(entry) {
  const payload = { context: signatureContext };
  for (const field of signedFields) {
    if (entry?.[field] === undefined) throw new Error(`registry entry is missing signed field ${field}`);
    payload[field] = entry[field];
  }
  return Buffer.from(canonicalJSONStringify(payload));
}

export function registrySignaturePayloadDigest(entry) {
  return `sha256:${createHash('sha256').update(registrySignaturePayload(entry)).digest('hex')}`;
}

export function signRegistryEntry(entry, privateKeyPEM, keyID) {
  if (!/^[a-z0-9]+(?:[._-][a-z0-9]+)*$/.test(keyID)) throw new Error('key_id is invalid');
  const privateKey = createPrivateKey(privateKeyPEM);
  if (privateKey.asymmetricKeyType !== 'ed25519') throw new Error('private key must be Ed25519');
  return {
    status: 'verified',
    algorithm: 'ed25519',
    key_id: keyID,
    value: sign(null, registrySignaturePayload(entry), privateKey).toString('base64'),
  };
}

export function verifyRegistryEntrySignature(entry, trustStore) {
  const signature = entry?.signature;
  if (signature?.status !== 'verified') throw new Error('registry signature status is not verified');
  if (signature.algorithm !== 'ed25519') throw new Error('registry signature algorithm must be ed25519');

  const matchingKeys = (trustStore?.keys ?? []).filter(key => key?.key_id === signature.key_id);
  if (matchingKeys.length !== 1) throw new Error(`trusted key ${JSON.stringify(signature.key_id)} must resolve exactly once`);
  const trustedKey = matchingKeys[0];
  if (trustedKey.status !== 'active') throw new Error(`trusted key ${signature.key_id} is not active`);
  if (trustedKey.algorithm !== 'ed25519') throw new Error(`trusted key ${signature.key_id} is not Ed25519`);

  const publicKey = createPublicKey(trustedKey.public_key);
  if (publicKey.asymmetricKeyType !== 'ed25519') throw new Error(`trusted key ${signature.key_id} is not an Ed25519 public key`);
  const signatureBytes = strictBase64(signature.value);
  if (signatureBytes.length !== 64) throw new Error('Ed25519 signature must be 64 bytes');
  if (!verify(null, registrySignaturePayload(entry), publicKey, signatureBytes)) {
    throw new Error('registry signature verification failed');
  }

  return {
    key_id: signature.key_id,
    payload_digest: registrySignaturePayloadDigest(entry),
  };
}

export function publicKeyFingerprint(key) {
  const publicKey = key?.type === 'public' ? key : createPublicKey(key);
  if (publicKey.asymmetricKeyType !== 'ed25519') throw new Error('public key must be Ed25519');
  const der = publicKey.export({ type: 'spki', format: 'der' });
  return `sha256:${createHash('sha256').update(der).digest('hex')}`;
}

function canonicalJSONStringify(value) {
  return JSON.stringify(canonicalValue(value));
}

function canonicalValue(value) {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return value;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (Array.isArray(value)) return value.map(canonicalValue);
  if (typeof value === 'object') {
    const result = {};
    for (const key of Object.keys(value).sort()) {
      if (value[key] === undefined) throw new Error(`canonical JSON does not support undefined at ${key}`);
      result[key] = canonicalValue(value[key]);
    }
    return result;
  }
  throw new Error(`canonical JSON does not support ${typeof value}`);
}

function strictBase64(value) {
  if (typeof value !== 'string' || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) {
    throw new Error('signature value is not valid base64');
  }
  return Buffer.from(value, 'base64');
}
