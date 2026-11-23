import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { mkdir, mkdtemp, realpath, rm, writeFile } from 'node:fs/promises';
import { loadClient, closeClientHarness } from './support/client-harness.js';

after(closeClientHarness);

const hostRequire = createRequire(
  process.env.FLYWHEEL_TEST_DSH_HOST_DIR
    ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json')
    : new URL('../package.json', import.meta.url),
);
const load = name => import(pathToFileURL(hostRequire.resolve(name)));
const { Context } = await load('@deepseek-ai/cordis');
const { default: TypertRegistry } = await load('@deepseek-ai/dsh-typert-registry');
const { default: Gateway } = await load('@deepseek-ai/dsh-api-gateway');
const GatewayClient = await load('@deepseek-ai/dsh-api-gateway/client');

const source = name =>
  process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR
    ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR, 'dist', name.replace(/\.js$/, '.mjs')))
    : process.env.FLYWHEEL_TEST_DSH_HOST_DIR
      ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, name))
      : new URL('../src/' + name, import.meta.url);

const { ConsoleReadService } = await import(source('console-read.js'));
const { ControlNotDispatchedError } = await import(source('control.js'));

test('Direct Typert Remote flywheelConsole serves query, act, design-candidates, ux-review, and listRepoImages without an active agent session', async t => {
  const ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(TypertRegistry);
  ctx.provide('remoteAccess', { decide: () => ({ kind: 'allow' }) });
  await ctx.plugin(Gateway);

  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-direct-rpc-')));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const taskDir = join(cwd, 'docs', 'demo-task');
  await mkdir(join(taskDir, 'design-candidates'), { recursive: true });
  await mkdir(join(taskDir, 'reports'), { recursive: true });
  await mkdir(join(cwd, 'assets'), { recursive: true });
  await writeFile(join(taskDir, 'design-candidates', 'rev-000001-primary.md'), '# Carbon Draft\n');
  await writeFile(join(taskDir, 'design-candidates', 'rev-000001-secondary.md'), '# Opus Draft\n');
  await writeFile(join(taskDir, 'design-candidates', 'rev-000001-meta.json'), JSON.stringify({ revision: 1, primaryProfile: 'carbon' }));
  await writeFile(join(taskDir, 'reports', 'M01-T01.ux-review.json'), JSON.stringify({ verdict: 'pass', summary: 'Crisp microcopy', findings: [] }));
  await writeFile(join(cwd, 'assets', 'wireframe.png'), Buffer.from([0x89, 0x50, 0x4e, 0x47]));

  const calls = [];
  const modelRequest = { version: 1, harness: 'antigravity', channel: 'google-native' };
  const catalog = patch => ({
    ...modelRequest,
    status: 'available',
    source: 'agy models',
    scope: 'native-client-catalog',
    models: [{ id: 'gemini-pro-orchestrator-neon', label: 'Orchestrator Neon', efforts: [''], defaultEffort: '' }],
    truncated: false,
    inferencePerformed: false,
    accountEntitlement: 'unknown',
    limitations: ['Catalog presence does not establish account entitlement.'],
    ...patch,
  });
  let modelReport = catalog();

  const control = async (input, agent, signal, opId, humanPlanningAction, humanSessionAction, overrideHostSessionID) => {
    assert.equal(agent, undefined, 'Direct UI RPC must not require an LLM agent');
    assert.ok(signal instanceof AbortSignal);
    calls.push({ input, opId, humanPlanningAction, humanSessionAction, overrideHostSessionID });
    return {
      protocol: 1,
      ok: true,
      running: false,
      state: null,
      taskDir: input.taskDir ?? '',
      cwd: input.cwd ?? cwd,
      ...(input.action === 'config-edit' ? { editor: { scope: input.editor.scope, revision: 'rev-1', values: {}, defaults: {} } } : {}),
      ...(input.action === 'models' ? { modelCatalog: modelReport } : {}),
      ...(input.action === 'runs' ? { runs: [{ taskDir, cwd, slug: 'demo-task', status: 'ready' }] } : {}),
    };
  };

  new ConsoleReadService(
    ctx,
    control,
    (input, error) => ({
      protocol: 1,
      type: 'flywheel-control-error',
      action: input?.action ?? null,
      dispatch: error instanceof ControlNotDispatchedError ? 'not-dispatched' : 'unknown',
      message: error.message,
    }),
    cwd,
  );

  const peer = { id: 'ui-peer', ctx, principal: { kind: 'owner' }, signal: new AbortController().signal, dispose: async () => {} };

  const clientCtx = new Context();
  t.after(() => clientCtx.fiber.dispose());
  await clientCtx.plugin(TypertRegistry);
  clientCtx.provide('connection', {
    rpc: {
      call: async (route, endpoint, payload, signal) => {
        assert.equal(route, '/api');
        const [namespace, method] = endpoint.split('/');
        assert.equal(namespace, 'flywheelConsole');
        return {
          ok: true,
          value: await ctx.typertGateway.invoke({
            namespace,
            method,
            args: JSON.parse(JSON.stringify(payload.args)),
            peer: { ...peer, signal },
          }),
        };
      },
      open: async function* () {},
    },
    start: () => ({ stop() {} }),
    registerGenerationSource: () => () => {},
  });
  await clientCtx.plugin(GatewayClient);

  const registered = new Map();
  clientCtx.provide('locale', { register: () => () => {}, bind: () => key => key });
  clientCtx.provide('slots', {
    inject: (_name, register) => register(),
    register: (def, Component) => {
      registered.set(def.name, { def, Component });
      return () => registered.delete(def.name);
    },
  });

  await clientCtx.plugin(loadClient());
  assert.ok(registered.has('main'));
  const { store } = registered.get('main').def.inject();

  // 1. Direct query: runs + design-candidates + ux-review + listRepoImages
  const runs = await store.refreshRuns();
  assert.equal(store.getSnapshot().runsError, null, 'refreshRuns should succeed through real GatewayClient');
  assert.equal(runs.length, 1);
  assert.equal(runs[0].taskDir, taskDir);

  await store.loadDesignCandidates();
  assert.equal(store.getSnapshot().designCandidates.length, 1);
  assert.match(store.getSnapshot().designCandidates[0].primary, /Carbon Draft/);
  assert.match(store.getSnapshot().designCandidates[0].secondary, /Opus Draft/);

  await store.loadUXReview('M01-T01');
  assert.equal(store.getSnapshot().uxReviews['M01-T01'].verdict, 'pass');

  const images = await store.loadRepoImages(cwd);
  assert.deepEqual(Array.from(images, f => f.path), ['assets/wireframe.png']);

  // 2. Model catalog validation rejects tampered/invented metadata
  const okCatalog = await store.loadModelCatalog(modelRequest);
  assert.equal(okCatalog.models[0].id, 'gemini-pro-orchestrator-neon');
  for (const patch of [
    { harness: 'codex-pro' },
    { channel: 'other' },
    { inferencePerformed: true },
    { accountEntitlement: 'confirmed' },
    { models: [{ id: 'invented', extraAuthority: true }] },
  ]) {
    modelReport = catalog(patch);
    const bad = await store.loadModelCatalog(modelRequest);
    assert.equal(bad, undefined);
    assert.match(store.getSnapshot().error.message, /invalid model catalog/);
  }
  modelReport = catalog();

  // 3. Direct human-only act() mints host-owned dsh approval proof & session abandon proof
  const digest = 'a'.repeat(64);
  await store.remote.act({
    action: 'planning',
    taskDir,
    cwd,
    planning: {
      action: 'approve',
      'request-id': 'approve-ui-1',
      revision: 3,
      design: { revision: 1, digest },
      start: true,
    },
  });
  const approveCall = calls.at(-1);
  assert.equal(approveCall.humanPlanningAction.kind, 'dsh');
  assert.equal(approveCall.humanPlanningAction.start, true);
  assert.equal(approveCall.humanPlanningAction.design.digest, digest);
  assert.ok(approveCall.opId.startsWith('ui:ui-'));

  await store.remote.act({
    action: 'session',
    taskDir,
    cwd,
    session: {
      action: 'abandon',
      key: 'task:M01-T01',
      invocation: 'inv-42',
      revision: 4,
      reason: 'Operator confirmed stuck worker',
    },
  });
  const abandonCall = calls.at(-1);
  assert.equal(abandonCall.humanSessionAction.kind, 'dsh');
  assert.equal(abandonCall.humanSessionAction.invocation, 'inv-42');

  // 4. Rejects unsupported query / act actions cleanly before dispatch
  await assert.rejects(
    store.remote.query({ action: 'start', taskDir }),
    /Invalid flywheelConsole\.query|Unsupported/,
  );
  await assert.rejects(
    store.remote.act({ action: 'runs' }),
    /Invalid flywheelConsole\.act|Unsupported/,
  );

  // 5. Verify registered Direct Typert descriptors have no agent lookup
  const queryDesc = clientCtx.typert.remotes.get('flywheelConsole/query');
  const actDesc = clientCtx.typert.remotes.get('flywheelConsole/act');
  const imgDesc = clientCtx.typert.remotes.get('flywheelConsole/listRepoImages');
  assert.equal(queryDesc.invocation.kind, 'direct');
  assert.equal(actDesc.invocation.kind, 'direct');
  assert.equal(imgDesc.invocation.kind, 'direct');
  assert.equal(queryDesc.parameters[0].wire, 'input');
  assert.equal(imgDesc.parameters[0].acceptsUndefined, true);
});
