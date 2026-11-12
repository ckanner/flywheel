// Only the desktop Host owns Workspace mutations. SDK workers must never open
// a second writer on the desktop's workspace storage.
const workerId = /^flywheel-[a-f0-9-]{36}$/;

export async function reconcileWorkspaces(ctx, response) {
  const registry = ctx.get?.('workspaceRegistry');
  const persistence = ctx.get?.('sessionPersistence');
  if (!registry || !persistence) return;
  const project = response?.state?.project?.['workspace-cwd'];
  if (project) await registry.create(project);
  const groups = new Map(registry.list().map(workspace => [workspace.path, workspace]));
  if (!groups.size) return;
  // Public metadata listing, never a scan of project directories or session
  // event bodies. attachSession independently validates the persisted header.
  for (const snapshot of await persistence.list()) {
    const header = snapshot.header;
    if (!workerId.test(header.id)) continue;
    const workspace = groups.get(header.cwd);
    if (workspace && !workspace.sessionIds.includes(header.id)) {
      await workspace.attachSession(header.id);
    }
  }
}

export function watchWorkspaces(ctx) {
  // SDK profile/older compositions may omit Workspace entirely. Keep the
  // controller available, and mount reconciliation when the public services exist.
  ctx.inject?.(['workspaceRegistry', 'sessionPersistence'], (scope) => {
    let busy = false;
    const refresh = async () => {
      if (busy) return;
      busy = true;
      try { await reconcileWorkspaces(scope); }
      catch (error) { scope.logger.warn('Flywheel 工作区归属未同步：%s', error.message); }
      finally { busy = false; }
    };
    const timer = setInterval(refresh, 30000);
    timer.unref?.();
    scope.on('dispose', () => clearInterval(timer));
    return refresh();
  });
}
