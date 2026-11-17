#!/usr/bin/env node
// Manual installed-runtime check. No prompt, title generation, credentials or
// existing user home is used. All model traffic targets a local rejecting server.
import assert from 'node:assert/strict';
import http from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, realpath, rm, writeFile } from 'node:fs/promises';
import { join, isAbsolute } from 'node:path';
import { tmpdir } from 'node:os';
import { prepareKimiPrintSession } from '../src/kimi-print-session.js';
import { OwnedKimiRuntime, readKimiMainAgent, inspectKimiSchemas } from '../src/kimi-native-runtime.js';

if (process.argv.length !== 3 || !isAbsolute(process.argv[2])) {
  throw new Error('Usage: node plugins/dsh/scripts/verify-kimi-print-history.mjs /absolute/path/to/kimi');
}
const binary = await realpath(process.argv[2]);
const root = await mkdtemp(join(tmpdir(), 'flywheel-kimi-history-'));
const home = join(root, 'home'), cwd = join(root, 'project');
const report = { verified: false, modelRequests: 0, requests: [] };
const offline = http.createServer((_request, response) => {
  report.modelRequests++;
  response.writeHead(503); response.end('This verification never permits inference');
});
await new Promise(resolve => offline.listen(0, '127.0.0.1', resolve));
const originalRequest = OwnedKimiRuntime.prototype.request;
OwnedKimiRuntime.prototype.request = function (path, body, options) {
  report.requests.push({ method: body === undefined ? 'GET' : 'POST', path });
  return originalRequest.call(this, path, body, options);
};
let reopened;
try {
  await mkdir(home); await mkdir(cwd);
  await writeFile(join(home, 'config.toml'), `default_model = "fixture-model"
default_permission_mode = "manual"
default_plan_mode = true
telemetry = false
auto_session_title = false
[providers.offline]
type = "openai"
base_url = "http://127.0.0.1:${offline.address().port}/v1"
api_key = "offline-no-inference"
[models.fixture-model]
provider = "offline"
model = "offline-fixture"
max_context_size = 32768
capabilities = ["thinking", "tool_use"]
support_efforts = ["low", "high"]
default_effort = "high"
`);
  // Do not inherit account credentials, provider overrides or a user's config.
  const env = { PATH: process.env.PATH, LANG: 'C.UTF-8', KIMI_CODE_HOME: home,
    FLYWHEEL_KIMI_BIN: binary, KIMI_CODE_NO_AUTO_UPDATE: '1' };
  const version = await promisify(execFile)(binary, ['--version'], { env, timeout: 5000, maxBuffer: 65536 });
  report.runtimeVersion = version.stdout.trim();
  const input = { type: 'prepare-print', protocol: 1, requestId: 'verify-print-history', cwd, model: 'fixture-model',
    reasoningEffort: 'high', sessionTitle: 'Flywheel · review · Native history compatibility', timeoutMs: 30_000 };
  report.prepared = await prepareKimiPrintSession(input, { env });
  assert.equal(report.prepared.status, 'ok', JSON.stringify(report.prepared));
  assert.equal(report.prepared.cleanupConfirmed, true);
  assert.equal(report.prepared.inferenceDispatched, false);

  reopened = new OwnedKimiRuntime(env, 'agent');
  await reopened.start(AbortSignal.timeout(20_000));
  report.identity = reopened.identity;
  const openapi = await reopened.request('/openapi.json', undefined, { raw: true });
  const asyncapi = await reopened.request('/asyncapi.json', undefined, { raw: true });
  try {
    inspectKimiSchemas(openapi, asyncapi, 'agent');
    report.nativeSchema = { supported: true, inferenceTested: false };
  } catch (cause) {
    assert.equal(cause.code, 'UNSUPPORTED_SCHEMA');
    report.nativeSchema = { supported: false, code: cause.code, message: cause.message, inferenceTested: false };
  }
  const path = '/api/v1/sessions/' + encodeURIComponent(report.prepared.sessionId);
  const list = await reopened.request('/api/v1/sessions');
  report.listed = list.items.find(item => item.id === report.prepared.sessionId);
  assert.equal(report.listed.title, input.sessionTitle);
  assert.equal(report.listed.metadata.flywheel_print_invocation, input.requestId);
  assert.equal(report.listed.metadata.cwd, await realpath(cwd));
  report.beforeLazyLoad = await reopened.request(path);
  report.reopenedAgent = await readKimiMainAgent(reopened, report.prepared.sessionId, asyncapi, AbortSignal.timeout(10_000));
  assert.equal(report.reopenedAgent.model, input.model);
  assert.equal(report.reopenedAgent.permission_mode, 'auto');
  assert.equal(report.reopenedAgent.plan_mode, false);
  assert.equal(report.reopenedAgent.status, 'idle');
  report.afterLazyLoad = await reopened.request(path);
  assert.equal(report.afterLazyLoad.agent_config.model, input.model);
  report.history = await reopened.request(path + '/messages?page_size=1');
  assert.deepEqual(report.history.items, []); assert.equal(report.history.has_more, false);
  report.queue = await reopened.request(path + '/prompts');
  assert.equal(report.queue.active, null); assert.deepEqual(report.queue.queued, []);
  assert.ok(!report.requests.some(row => row.method === 'POST' && (row.path.endsWith('/prompts') || row.path.endsWith('/title/generate'))));
  report.verified = true;
} catch (cause) {
  report.error = { code: cause.code || 'VERIFICATION_FAILED', message: cause.message };
  process.exitCode = 1;
} finally {
  try {
    if (reopened) {
      report.cleanup = await reopened.stop();
      assert.equal(report.cleanup.exited && report.cleanup.processesConfirmed, true);
    }
    assert.equal(report.modelRequests, 0);
  } catch (cause) {
    report.verified = false;
    report.cleanupError = { code: cause.code || 'VERIFICATION_FAILED', message: cause.message };
    process.exitCode = 1;
  }
  OwnedKimiRuntime.prototype.request = originalRequest;
  await new Promise(resolve => offline.close(resolve));
  await rm(root, { recursive: true, force: true });
  report.temporaryHomeRemoved = true;
  process.stdout.write(JSON.stringify(report, null, 2) + '\n');
}
