#!/usr/bin/env node

import { createPublicKey } from 'node:crypto';
import { readFile, realpath, stat } from 'node:fs/promises';
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';
import {
  publicKeyFingerprint,
  registrySignaturePayloadDigest,
  signRegistryEntry,
  verifyRegistryEntrySignature,
} from './lib/plugin-release.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pluginID = 'contentcloud-video-production';
const argumentsByName = parseArguments(process.argv.slice(2));
const privateKeyArgument = argumentsByName.get('--private-key');
const keyID = argumentsByName.get('--key-id');

if (!privateKeyArgument || !keyID) {
  fail('usage: node scripts/sign-plugin-release.mjs --private-key <outside-repo.pem> --key-id <trusted-key-id>');
}

const privateKeyPath = await realpath(resolve(process.cwd(), privateKeyArgument)).catch(error => {
  fail(`cannot resolve private key: ${error.message}`);
});
const relativePrivateKeyPath = relative(root, privateKeyPath);
if (relativePrivateKeyPath === '' || (!relativePrivateKeyPath.startsWith(`..${sep}`) && relativePrivateKeyPath !== '..' && !isAbsolute(relativePrivateKeyPath))) {
  fail('private key must be stored outside the repository');
}
if (process.platform !== 'win32' && ((await stat(privateKeyPath)).mode & 0o077) !== 0) {
  fail('private key permissions must not grant group or other access');
}

const registry = await readJSON('.agents/plugins/registry.json');
const trustStore = await readJSON('.agents/plugins/trusted-keys.json');
const entry = registry.entries?.find(candidate => candidate?.id === pluginID);
if (!entry) fail(`registry entry ${pluginID} was not found`);

const trustedKeys = (trustStore.keys ?? []).filter(key => key?.key_id === keyID);
if (trustedKeys.length !== 1 || trustedKeys[0].status !== 'active') {
  fail(`active trusted key ${JSON.stringify(keyID)} must exist exactly once`);
}

const privateKeyPEM = await readFile(privateKeyPath, 'utf8');
const derivedPublicKey = createPublicKey(privateKeyPEM);
if (publicKeyFingerprint(derivedPublicKey) !== publicKeyFingerprint(trustedKeys[0].public_key)) {
  fail(`private key does not match trusted key ${keyID}`);
}

const signature = signRegistryEntry(entry, privateKeyPEM, keyID);
verifyRegistryEntrySignature({ ...entry, signature }, trustStore);
process.stdout.write(`${JSON.stringify({
  plugin: `${entry.id}@${entry.version}`,
  payload_digest: registrySignaturePayloadDigest(entry),
  signature,
}, null, 2)}\n`);

async function readJSON(path) {
  return JSON.parse(await readFile(resolve(root, path), 'utf8'));
}

function parseArguments(args) {
  const result = new Map();
  for (let index = 0; index < args.length; index += 2) {
    const name = args[index];
    const value = args[index + 1];
    if (!name?.startsWith('--') || value === undefined || value.startsWith('--') || result.has(name)) {
      fail(`invalid argument sequence near ${JSON.stringify(name)}`);
    }
    result.set(name, value);
  }
  for (const name of result.keys()) {
    if (!['--private-key', '--key-id'].includes(name)) fail(`unsupported argument ${name}`);
  }
  return result;
}

function fail(message) {
  process.stderr.write(`${message}\n`);
  process.exit(1);
}
