// Required-validator failure is exercised even when CI has installed Ajv.
// The test-only ESM hook makes exactly that dependency unavailable; it does
// not replace validation with a permissive implementation or alter the skill.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const skillRoot = fileURLToPath(new URL('..', import.meta.url));
const examples = {
  workflow: 'agent-tool-call.workflow.json',
  sequence: 'cache-miss-request.sequence.json',
  dataflow: 'product-analytics.dataflow.json',
  lifecycle: 'agent-run.lifecycle.json',
  architecture: 'web-app.architecture.json',
};

function unavailableValidator(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'atlasflow-validator-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const loader = path.join(root, 'missing-validator.mjs');
  fs.writeFileSync(loader, `export async function resolve(specifier, context, nextResolve) {
    if (specifier === 'ajv/dist/2020.js') {
      const error = new Error('Required validator unavailable in this test process.');
      error.code = 'ERR_MODULE_NOT_FOUND';
      throw error;
    }
    return nextResolve(specifier, context);
  }`);
  const scratch = path.join(root, 'scratch');
  fs.mkdirSync(scratch);
  return {
    root,
    scratch,
    run(args) {
      return spawnSync(process.execPath, [path.join(skillRoot, 'bin/atlasflow.mjs'), ...args], {
        cwd: root, encoding: 'utf8', timeout: 10000, maxBuffer: 1 << 20,
        env: {
          ...process.env, NODE_OPTIONS: `--experimental-loader=${pathToFileURL(loader).href}`,
          TMPDIR: scratch, TMP: scratch, TEMP: scratch,
        },
      });
    },
  };
}

function assertBlocked(result) {
  assert.equal(result.error, undefined, 'must reject, not time out or fail to spawn');
  assert.equal(result.signal, null, 'must exit normally with a failure code');
  assert.notEqual(result.status, 0, 'missing mandatory validation must never succeed');
  assert.match(result.stderr, /schema validation is required/i);
  assert.doesNotMatch(result.stdout, /"ok"\s*:\s*true|^ok /m);
}

for (const [type, example] of Object.entries(examples)) {
  test(`${type}: missing validator cannot create or overwrite output`, t => {
    const f = unavailableValidator(t);
    const input = path.join(skillRoot, 'examples', example);
    const output = path.join(f.root, 'output.html');
    assertBlocked(f.run(['render', type, input, output]));
    assert.equal(fs.existsSync(output), false);
    fs.writeFileSync(output, 'previous-user-output');
    assertBlocked(f.run(['render', type, input, output]));
    assert.equal(fs.readFileSync(output, 'utf8'), 'previous-user-output');
  });
}

test('missing validator cannot report JSON validation success or retain temporary output', t => {
  const f = unavailableValidator(t);
  assertBlocked(f.run(['validate', 'workflow', path.join(skillRoot, 'examples', examples.workflow), '--json']));
  assert.deepEqual(fs.readdirSync(f.scratch), []);
});

test('missing validator cannot report an inspected layout', t => {
  const f = unavailableValidator(t);
  assertBlocked(f.run(['inspect', 'architecture', path.join(skillRoot, 'examples', examples.architecture)]));
});

test('help remains available without the renderer dependency', t => {
  const f = unavailableValidator(t);
  const result = f.run(['--help']);
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /atlasflow render <type>/);
});
