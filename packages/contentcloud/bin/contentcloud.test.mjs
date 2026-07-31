import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {chmod, mkdtemp, readFile, rm, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {promisify} from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);

test('update restarts an existing daemon through the newly installed binary', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'contentcloud-installer-test-'));
  try {
    const executable = join(directory, 'contentcloud-test');
    const log = join(directory, 'arguments.log');
    await writeFile(executable, '#!/bin/sh\nprintf "%s\\n" "$*" > "$CONTENTCLOUD_TEST_LOG"\n', {mode: 0o700});
    await chmod(executable, 0o700);
    await execFileAsync(process.execPath, [new URL('./contentcloud.js', import.meta.url).pathname, 'update'], {
      env: {...process.env, CONTENTCLOUD_BINARY_PATH: executable, CONTENTCLOUD_TEST_LOG: log},
    });
    assert.equal((await readFile(log, 'utf8')).trim(), '--json daemon restart --if-installed');
  } finally {
    await rm(directory, {recursive: true, force: true});
  }
});
