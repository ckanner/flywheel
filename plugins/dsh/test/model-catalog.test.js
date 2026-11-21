import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { modelCatalog, validateCatalogRequest, catalogEnvironment, MODEL_CATALOG_LIMIT } from '../src/model-catalog.js';
import { CatalogProcess } from '../src/model-catalog-process.js';
import { validModelCatalogRequest, validModelCatalogReport } from '../src/model-catalog-contract.js';

const binding = (harness, extra = {}) => ({ version: 1, harness, cwd: process.cwd(), ...extra });
const codexModel = (model = 'gpt-6.2') => ({ model, displayName: 'GPT 6.2', description: 'Native catalog',
  supportedReasoningEfforts: [{ reasoningEffort: 'medium' }, { reasoningEffort: 'high' }], defaultReasoningEffort: 'medium' });
const cleanEnv = { PATH: process.env.PATH, HOME: tmpdir() };

function fakeProcess({ text, pages = [], failure, requests = [], onClose = () => {} } = {}) {
  let page = 0;
  return (command, args, options) => {
    requests.push({ command, args, options });
    return {
      async text() { if (failure) throw failure; return text; },
      async request(method, params) {
        requests.push({ method, params });
        if (failure) throw failure;
        if (method === 'initialize') return {};
        assert.equal(method, 'model/list', 'catalog must never open a thread or turn');
        return pages[page++];
      },
      notify(method) { requests.push({ method }); assert.equal(method, 'initialized'); },
      async close() { onClose(); },
    };
  };
}

test('catalog accepts only bounded staged selectors and never accepts tasks or credentials', () => {
  for (const value of [null, [], binding('codex-pro', { version: 2 }), binding('bad harness'),
    binding('codex-pro', { cwd: 'relative' }), binding('codex-pro', { prompt: 'do work' }),
    binding('codex-pro', { provider: 'key\nsecret' }), binding('codex-pro', { channel: '' }),
    binding('codex-pro', { model: 'not-needed' }), binding('codex-pro', { command: 'anything' }),
    binding('codex-pro', { apiKey: 'secret' })]) assert.throws(() => validateCatalogRequest(value));
  assert.deepEqual(validateCatalogRequest(binding('dsh', { provider: 'chatgpt', channel: 'native' })), binding('dsh', { provider: 'chatgpt', channel: 'native' }));
});

test('metadata subprocesses get only local environment selectors', () => {
  assert.deepEqual(catalogEnvironment({ PATH: '/bin', CODEX_HOME: '/local/codex', HOME: '/local',
    OPENAI_API_KEY: 'secret', ANTHROPIC_API_KEY: 'secret', NODE_OPTIONS: '--require=/evil', FLYWHEEL_AGY_BIN: '/agy',
    ANTHROPIC_BASE_URL: 'https://secret', AGY_ADC_AUTH: 'secret' }), { PATH: '/bin', HOME: '/local', CODEX_HOME: '/local/codex' });
});

test('shared browser contract rejects stale identity, inference claims, duplicate rows and unbounded metadata', async () => {
  const request = { version: 1, harness: 'antigravity', channel: 'native' };
  assert.equal(validModelCatalogRequest(request), true);
  assert.equal(validModelCatalogRequest({ ...request, cwd: '/unexpected' }), false);
  assert.equal(validModelCatalogRequest({ ...request, provider: null }), false);
  const report = await modelCatalog({ ...request, cwd: process.cwd() }, { process: fakeProcess({ text: 'gemini-pro-orchestrator' }) });
  assert.equal(validModelCatalogReport(report, request), true);
  assert.equal(validModelCatalogReport(report, { ...request, channel: 'other' }), false);
  for (const change of [
    value => { value.extra = true; }, value => { value.source = 'static guesses'; }, value => { value.scope = 'account'; },
    value => { value.accountEntitlement = 'verified'; }, value => { value.inferencePerformed = true; },
    value => { value.truncated = null; }, value => { value.models = null; }, value => { value.models = Array(257).fill(value.models[0]); },
    value => { value.models.push(value.models[0]); }, value => { value.models[0].id = 'bad id'; },
    value => { value.models[0].extra = true; }, value => { value.models[0].label = 'invisible\u200b'; },
    value => { value.models[0].description = 'multi\nline'; }, value => { value.models[0].efforts = ['high', 'high']; },
    value => { value.models[0].efforts = ['unknown']; }, value => { value.models[0].defaultEffort = 'high'; },
    value => { value.error = { code: 'native', message: 'unexpected success error' }; },
    value => { value.limitations = ['']; }, value => { value.limitations = Array(17).fill('bounded'); },
    value => { value.status = 'unavailable'; },
  ]) {
    const invalid = structuredClone(report);
    change(invalid);
    assert.equal(validModelCatalogReport(invalid, request), false, JSON.stringify(invalid));
  }
  const failed = { ...report, status: 'unavailable', models: [], error: { code: 'CATALOG_UNAVAILABLE', message: 'Catalog unavailable.' } };
  assert.equal(validModelCatalogReport(failed, request), true);
  assert.equal(validModelCatalogReport({ ...failed, error: { ...failed.error, details: 'private' } }, request), false);
  const unsupportedRequest = { version: 1, harness: 'kimi' };
  const unsupported = await modelCatalog({ ...unsupportedRequest, cwd: process.cwd() });
  assert.equal(validModelCatalogReport(unsupported, unsupportedRequest), true);
  assert.equal(validModelCatalogReport({ ...unsupported, status: 'available' }, unsupportedRequest), false);
});

test('unsupported bindings never start a runtime and report the missing transport truthfully', async () => {
  for (const harness of ['kimi', 'kimi-native', 'dsh', 'pi', 'zcode', 'constructor', 'toString']) {
    const result = await modelCatalog(binding(harness), { process() { assert.fail('unsupported catalog must not start'); } });
    assert.equal(result.status, 'unsupported');
    assert.equal(result.source, 'unsupported');
    assert.deepEqual(result.models, []);
    assert.equal(result.accountEntitlement, 'unknown');
    assert.equal(result.inferencePerformed, false);
    assert.equal(validModelCatalogReport(result, { version: 1, harness }), true);
  }
});

test('Antigravity lists observed CLI IDs, preserves fixed effort, and closes the owned command', async () => {
  const requests = [];
  let closed = false;
  const result = await modelCatalog(binding('antigravity'), { env: { ...cleanEnv, FLYWHEEL_AGY_BIN: '/reviewed/agy' },
    process: fakeProcess({ requests, onClose() { closed = true; },
      text: 'Available models:\n\ngemini-4-hydrogen-medium  Hydrogen Medium\ngemini-pro-orchestrator  Carbon\ngemini-new-future-id\n' }) });
  assert.equal(result.status, 'available');
  assert.equal(result.source, 'agy models');
  assert.deepEqual(result.models.map(model => [model.id, model.efforts, model.defaultEffort]), [
    ['gemini-4-hydrogen-medium', ['medium'], 'medium'], ['gemini-pro-orchestrator', [''], ''], ['gemini-new-future-id', undefined, undefined],
  ]);
  assert.equal(closed, true);
  assert.equal(requests[0].command, '/reviewed/agy');
  assert.deepEqual(requests[0].args, ['models']);
});

test('Antigravity does not fabricate empty/duplicate output or unknown effort support', async () => {
  for (const text of ['', 'No models available.', 'gemini-4-neon-high\ngemini-4-neon-high\n']) {
    const result = await modelCatalog(binding('antigravity'), { process: fakeProcess({ text }) });
    assert.equal(result.status, 'unavailable');
    assert.deepEqual(result.models, []);
  }
  const unknown = await modelCatalog(binding('antigravity'), { process: fakeProcess({ text: 'gemini-4-carbon-xhigh\n' }) });
  assert.equal(unknown.models[0].efforts, undefined);
});

test('native catalogs make row truncation explicit', async () => {
  const result = await modelCatalog(binding('antigravity'), { process: fakeProcess({
    text: Array.from({ length: MODEL_CATALOG_LIMIT + 1 }, (_, i) => `gemini-model-${i}`).join('\n'),
  }) });
  assert.equal(result.status, 'available');
  assert.equal(result.models.length, MODEL_CATALOG_LIMIT);
  assert.equal(result.truncated, true);
});

test('Codex metadata uses only initialize and paginated model/list with the staged provider', async () => {
  const requests = [];
  const result = await modelCatalog(binding('codex-pro', { provider: 'openai', channel: 'subscription' }), { process: fakeProcess({ requests,
    pages: [{ data: [codexModel()], nextCursor: 'page-2' }, { data: [codexModel('gpt-6.2-fast')], nextCursor: null }] }) });
  assert.equal(result.status, 'available');
  assert.deepEqual(result.models.map(model => model.id), ['gpt-6.2', 'gpt-6.2-fast']);
  assert.deepEqual(result.models[0].efforts, ['medium', 'high']);
  assert.equal(result.models[0].defaultEffort, 'medium');
  assert.deepEqual(requests[0].args, ['app-server', '--stdio']);
  assert.deepEqual(requests.slice(1).map(item => item.method), ['initialize', 'initialized', 'model/list', 'model/list']);
  assert.equal(requests[3].params.modelProvider, 'openai');
  assert.equal(requests[4].params.cursor, 'page-2');
  assert.equal(result.accountEntitlement, 'unknown');
});

test('Codex rejects invalid rows, duplicate identifiers, bad pagination and contradictory efforts', async () => {
  for (const pages of [
    [{ data: 'bad' }], [{ data: [null] }], [{ data: [{ ...codexModel(), supportedReasoningEfforts: null }] }],
    [{ data: [{ ...codexModel(), model: 'private\nsecret' }] }],
    [{ data: [{ ...codexModel(), defaultReasoningEffort: 'max' }] }],
    [{ data: [{ ...codexModel(), supportedReasoningEfforts: [{ reasoningEffort: 'high' }, { reasoningEffort: 'high' }] }] }],
    [{ data: [codexModel(), codexModel()] }], [{ data: [codexModel()], nextCursor: '' }],
    [{ data: [], nextCursor: 'same' }, { data: [], nextCursor: 'same' }],
  ]) {
    const result = await modelCatalog(binding('codex-pro'), { process: fakeProcess({ pages }) });
    assert.equal(result.status, 'unavailable');
    assert.deepEqual(result.models, []);
  }
});

test('Codex stops at the row and page bound without claiming completeness', async () => {
  const all = Array.from({ length: MODEL_CATALOG_LIMIT }, (_, i) => codexModel(`model-${i}`));
  for (const pages of [
    [{ data: all, nextCursor: 'more' }],
    Array.from({ length: 8 }, (_, i) => ({ data: [codexModel(`model-${i}`)], nextCursor: `page-${i}` })),
  ]) {
    const result = await modelCatalog(binding('codex-pro'), { process: fakeProcess({ pages }) });
    assert.equal(result.status, 'available');
    assert.equal(result.truncated, true);
  }
});

test('catalog never relays exceptions, stderr or provider error details', async () => {
  let closed = false;
  const result = await modelCatalog(binding('codex-pro'), { process: fakeProcess({
    failure: new Error('PRIVATE_TOKEN=sensitive-value'), onClose() { closed = true; },
  }) });
  assert.equal(result.status, 'unavailable');
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE_TOKEN|sensitive-value/);
  assert.equal(closed, true);
  const invalid = await modelCatalog(binding('antigravity'), { env: { FLYWHEEL_AGY_BIN: 'agy && login' }, process() { assert.fail('shell text must not spawn'); } });
  assert.equal(invalid.status, 'unavailable');
});

test('cancellation before probing starts no native process', async () => {
  const result = await modelCatalog(binding('codex-pro'), { signal: AbortSignal.abort(), process() { assert.fail('cancelled probe started'); } });
  assert.equal(result.status, 'unavailable');
  assert.equal(result.error.code, 'CANCELLED');
});

test('Claude initialization holds empty input, suppresses hooks/persistence, and preserves only verified effort metadata', async () => {
  let options, prompt, closed = false;
  const result = await modelCatalog(binding('claude-code-pro'), { loadClaude: async () => ({ query(input) {
    ({ options, prompt } = input);
    return { async supportedModels() { return [{ value: 'sonnet', displayName: 'Sonnet', supportedEffortLevels: ['medium', 'high'] }]; }, close() { closed = true; } };
  } }) });
  assert.equal(result.status, 'available');
  assert.equal(options.persistSession, false);
  assert.equal(options.settings.disableAllHooks, true);
  assert.deepEqual(options.settingSources, []);
  assert.deepEqual(options.tools, []);
  assert.deepEqual(options.plugins, []);
  assert.deepEqual(options.mcpServers, {});
  assert.equal(options.extraArgs['strict-mcp-config'], null);
  assert.equal(closed, true);
  assert.deepEqual(await prompt.next(), { value: undefined, done: true });
  assert.deepEqual(result.models[0].efforts, ['medium', 'high']);
  assert.equal(result.models[0].defaultEffort, undefined);
  assert.match(result.limitations.join(' '), /settings.*disabled/);
});

test('missing/failed Claude SDK has an explicit unavailable catalog', async () => {
  for (const loadClaude of [async () => ({}), async () => { throw Error('private module path'); },
    async () => ({ query() { return { async supportedModels() { throw Error('private account'); }, close() {} }; } }),
    async () => ({ query() { return { async supportedModels() { return [{ value: 'sonnet', displayName: '', supportedEffortLevels: ['high'] }]; }, close() {} }; } })]) {
    const result = await modelCatalog(binding('claude-code-pro'), { loadClaude });
    assert.equal(result.status, 'unavailable');
    assert.doesNotMatch(JSON.stringify(result), /private/);
  }
});

async function fixture(t, source) {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-model-catalog-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const path = join(cwd, 'native.mjs');
  await writeFile(path, '#!/usr/bin/env node\n' + source, { mode: 0o755 });
  return { cwd, path, env: { PATH: process.env.PATH, HOME: cwd } };
}

test('real SDK sends only initialize metadata to a fake CLI, with no user turn or settings writes', async t => {
  const f = await fixture(t, `import {createInterface} from 'node:readline';
import {appendFileSync,writeFileSync} from 'node:fs';
writeFileSync('argv.json', JSON.stringify(process.argv.slice(2)));
const lines=createInterface({input:process.stdin});
lines.on('line',line=>{
  const frame=JSON.parse(line);appendFileSync('frames.jsonl',line+'\\n');
  if(frame.type!=='control_request'||frame.request.subtype!=='initialize')process.exit(73);
  process.stdout.write(JSON.stringify({type:'control_response',response:{subtype:'success',request_id:frame.request_id,response:{commands:[],agents:[],models:[{value:'sonnet',displayName:'Sonnet',description:'native',supportedEffortLevels:['medium','high']}],account:{email:'private-account@example.invalid'},claude_code_version:'2.1.326'}}})+'\\n');
});
lines.on('close',()=>process.exit(0));`);
  const result = await modelCatalog(binding('claude-code-pro', { cwd: f.cwd }), { env: { ...f.env, FLYWHEEL_CLAUDE_BIN: f.path }, timeoutMs: 3000 });
  assert.equal(result.status, 'available', JSON.stringify(result));
  const frames = (await readFile(join(f.cwd, 'frames.jsonl'), 'utf8')).trim().split('\n').map(JSON.parse);
  assert.deepEqual(frames.map(frame => frame.request?.subtype), ['initialize']);
  assert.doesNotMatch(JSON.stringify(result), /private-account/);
  const argv = JSON.parse(await readFile(join(f.cwd, 'argv.json'), 'utf8'));
  assert.ok(argv.includes('--no-session-persistence'));
  assert.ok(argv.includes('--strict-mcp-config'));
  const settings = JSON.parse(argv[argv.indexOf('--settings') + 1]);
  assert.equal(settings.disableAllHooks, true);
});

test('owned metadata transport handles JSON-RPC without starting a thread', async t => {
  const f = await fixture(t, `import {createInterface} from 'node:readline';
const lines=createInterface({input:process.stdin});
lines.on('line',line=>{const f=JSON.parse(line);if(f.method==='initialized')return;
if(!['initialize','model/list'].includes(f.method))process.exit(74);
const result=f.method==='initialize'?{}:{data:[${JSON.stringify(codexModel())}],nextCursor:null};
process.stdout.write(JSON.stringify({id:f.id,result})+'\\n');});`);
  const result = await modelCatalog(binding('codex-pro', { cwd: f.cwd }), { env: { ...f.env, FLYWHEEL_CODEX_BIN: f.path }, timeoutMs: 2000 });
  assert.equal(result.status, 'available');
  assert.equal(result.models[0].id, 'gpt-6.2');
});

test('owned metadata transport rejects broken frames, errors and never returns raw diagnostics', async t => {
  for (const source of [
    `process.stdout.write('not json\\n');setInterval(()=>{},1000);`,
    `process.stdout.write(JSON.stringify({id:99,result:{}})+'\\n');setInterval(()=>{},1000);`,
    `process.stdout.write(JSON.stringify({id:1,method:'account/login/start'})+'\\n');setInterval(()=>{},1000);`,
    `process.stderr.write('SECRET_TOKEN=sensitive-value');process.exit(3);`,
    `import{createInterface}from'node:readline';createInterface({input:process.stdin}).on('line',()=>process.stdout.write(JSON.stringify({id:1,error:{message:'SECRET_TOKEN=sensitive-value'}})+'\\n'));`,
  ]) {
    const f = await fixture(t, source);
    const result = await modelCatalog(binding('codex-pro', { cwd: f.cwd }), { env: { ...f.env, FLYWHEEL_CODEX_BIN: f.path }, timeoutMs: 1000 });
    assert.equal(result.status, 'unavailable');
    assert.doesNotMatch(JSON.stringify(result), /SECRET_TOKEN|sensitive-value/);
  }
});

test('owned metadata transport cancels bounded output and silent hangs', async t => {
  for (const [source, expected] of [
    [`process.stdout.write('x'.repeat(1024*1024+1));setInterval(()=>{},1000);`, 'OUTPUT_LIMIT'],
    [`setInterval(()=>{},1000);`, 'CATALOG_TIMEOUT'],
  ]) {
    const f = await fixture(t, source);
    const result = await modelCatalog(binding('antigravity', { cwd: f.cwd }), { env: { ...f.env, FLYWHEEL_AGY_BIN: f.path }, timeoutMs: 200 });
    assert.equal(result.status, 'unavailable');
    assert.equal(result.error.code, expected);
    assert.equal(result.truncated, expected === 'OUTPUT_LIMIT');
  }
});

test('metadata text decoding preserves split Unicode and rejects malformed UTF-8', async t => {
  const f = await fixture(t, `const bytes=Buffer.from('模型');process.stdout.write(bytes.subarray(0,1));setTimeout(()=>{process.stdout.write(bytes.subarray(1));},5);`);
  const valid = new CatalogProcess(f.path, [], { cwd: f.cwd, env: f.env, timeoutMs: 1000 });
  assert.equal(await valid.text(), '模型');
  await valid.close();
  for (const data of ['[0xff]', '[0xe6]']) {
    const bad = await fixture(t, `process.stdout.write(Buffer.from(${data}));`);
    const invalid = new CatalogProcess(bad.path, [], { cwd: bad.cwd, env: bad.env, timeoutMs: 1000 });
    await assert.rejects(invalid.text(), /CATALOG_PROTOCOL/);
    await invalid.close();
  }
});

test('read-only worker rejects bad requests and emits one metadata report for unsupported bindings', () => {
  const worker = fileURLToPath(new URL('../src/model-catalog-worker.js', import.meta.url));
  for (const input of ['{}', '{}\n{}', JSON.stringify(binding('dsh', { prompt: 'should never execute' })), 'x'.repeat(8193)]) {
    const result = spawnSync(process.execPath, [worker], { input, encoding: 'utf8', env: cleanEnv, timeout: 2000 });
    assert.equal(result.status, 1);
    assert.equal(JSON.parse(result.stdout).error.code, 'INVALID_REQUEST');
  }
  const result = spawnSync(process.execPath, [worker], { input: JSON.stringify(binding('dsh')), encoding: 'utf8', env: cleanEnv, timeout: 2000 });
  assert.equal(result.status, 0);
  assert.equal(result.stdout.trim().split('\n').length, 1);
  assert.equal(JSON.parse(result.stdout).status, 'unsupported');
});

test('transport missing executable and abort still settle cleanly', async () => {
  const missing = new CatalogProcess('/does-not-exist/flywheel-catalog', [], { cwd: process.cwd(), env: cleanEnv, timeoutMs: 100 });
  await assert.rejects(missing.text());
  await missing.close();
  const stopped = new CatalogProcess(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { cwd: process.cwd(), env: cleanEnv, signal: AbortSignal.abort(), timeoutMs: 1000 });
  await assert.rejects(stopped.text());
  await stopped.close();
});
