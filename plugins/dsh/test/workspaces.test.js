import test from 'node:test';
import assert from 'node:assert/strict';
import { reconcileWorkspaces, watchWorkspaces } from '../src/workspaces.js';
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Context } from '@deepseek-ai/cordis';
import Storage from '@deepseek-ai/dsh-storage';
import * as StorageJSON from '@deepseek-ai/dsh-storage-json';
import * as Domain from '@deepseek-ai/dsh-storage-domain';
import WorkspaceRegistry from '@deepseek-ai/dsh-workspace';
import * as Persistence from '@deepseek-ai/dsh-session-persistence-jsonl';
import { runWorker } from '../src/bridge.js';

test('published Workspace registry attaches an out-of-process SDK session and preserves membership after restart', async t => {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-public-workspace-')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const project = join(root, 'project'), unit = join(root, 'unit'), history = join(root, 'history');
  await Promise.all([mkdir(project), mkdir(unit), mkdir(history)]);
  async function host() {
    const ctx = new Context();
    await ctx.plugin(Storage);
    await ctx.plugin(StorageJSON, { root: join(root, 'storage') });
    await ctx.plugin(Domain, { backend: 'json', routes: {} });
    await ctx.plugin(Persistence, { root: history, compression: 'none' });
    await ctx.plugin(WorkspaceRegistry);
    return ctx;
  }
  const ctx = await host();
  t.after(() => ctx.fiber.dispose());
  const workspace = await ctx.workspaceRegistry.create(project);
  const result = await runWorker({ type: 'start', protocol: 2, persistent: true, requestId: 'public-workspace',
    cwd: unit, workspaceCwd: project, provider: 'offline-fixture', model: 'test-model',
    permissionMode: 'danger-full-access', prompt: 'offline association check', timeoutMs: 10000 }, {
    env: { PATH: process.env.PATH, TEST_DSH_HISTORY: history,
      FLYWHEEL_DSH_BIN: fileURLToPath(new URL('./fixtures/persisted-sdk.js', import.meta.url)) },
    emit: () => {}, stderr: () => {},
  });
  assert.equal(result.status, 'ok', JSON.stringify(result));
  assert.deepEqual(workspace.sessionIds, []);
  await reconcileWorkspaces(ctx);
  assert.deepEqual(workspace.sessionIds, [result.sessionId]);
  assert.equal(ctx.workspaceRegistry.list().length, 1, 'execution Worktree must not become a desktop project');
  await ctx.fiber.dispose();
  const restored = await host();
  t.after(() => restored.fiber.dispose());
  assert.deepEqual(restored.workspaceRegistry.list().map(w => ({ path: w.path, sessions: w.sessionIds })),
    [{ path: project, sessions: [result.sessionId] }]);
});

test('desktop registers SDK sessions with an existing project, never a worktree or foreign session', async () => {
  const id = 'flywheel-11111111-1111-4111-8111-111111111111';
  const attached = [], created = [];
  const workspace = { path: '/project', sessionIds: [], attachSession: async id => { attached.push(id); workspace.sessionIds.push(id); } };
  const services = {
    workspaceRegistry: { create: async path => created.push(path), list: () => [workspace] },
    sessionPersistence: { list: async () => [
      { header: { id, cwd: '/project' } },
      { header: { id: id.replace('1111', '2222'), cwd: '/separate-worktree' } },
      { header: { id: 'unrelated', cwd: '/project' } },
    ] },
  };
  const ctx = { get: key => services[key] };
  await reconcileWorkspaces(ctx, { state: { project: { 'workspace-cwd': '/project' } } });
  await reconcileWorkspaces(ctx);
  assert.deepEqual(attached, [id]);
  assert.deepEqual(created, ['/project']);
  await reconcileWorkspaces({});
  workspace.attachSession = async () => { throw Error('header mismatch'); };
  workspace.sessionIds = [];
  await assert.rejects(reconcileWorkspaces(ctx), /header mismatch/);
});

test('workspace watcher is optional, runs once immediately, and releases its interval', async () => {
  watchWorkspaces({});
  let disposed, reads = 0;
  const services = {
    workspaceRegistry: { list: () => [{ path: '/project', sessionIds: [] }] },
    sessionPersistence: { list: async () => { reads++; return []; } },
  };
  let done;
  watchWorkspaces({ inject: (keys, fn) => {
    assert.deepEqual(keys, ['workspaceRegistry', 'sessionPersistence']);
    done = fn({ get: key => services[key], on: (event, fn) => { assert.equal(event, 'dispose'); disposed = fn; } });
  } });
  await done;
  disposed();
  assert.equal(reads, 1);
});
