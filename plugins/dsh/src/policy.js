// Loaded only in the independently owned SDK process, never in the desktop host.
// Public rc.5/rc.1 Cordis services; the worker verifies this acknowledgement before prompt().
import { link, realpath, stat, unlink, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute } from 'node:path';

export const inject = ['sandboxPolicy', 'approval', 'shell'];

export async function apply(ctx, config) {
  const { permissionMode, cwd, receiptPath, requestId, sessionId, nonce } = config;
  const fullAccess = permissionMode === 'danger-full-access';
  if (!['read-only', 'workspace-write', 'danger-full-access'].includes(permissionMode) || typeof cwd !== 'string' ||
      ctx.shell.sandboxMode === undefined) {
    throw new Error('Flywheel requires a sandbox-capable DSH shell and explicit permission policy');
  }
  if (typeof receiptPath !== 'string' || !isAbsolute(receiptPath) ||
      [requestId, sessionId, nonce].some(value => typeof value !== 'string' || value.trim() === '' || value.includes('\0'))) {
    throw new Error('Flywheel policy requires an owned, correlated admission receipt');
  }
  const parent = dirname(receiptPath);
  const owner = await stat(parent);
  if (await realpath(parent) !== parent || !owner.isDirectory() || (owner.mode & 0o077) !== 0) {
    throw new Error('Flywheel policy receipt must be inside its private physical directory');
  }
  // Probe the official runner. Restricted modes must prove full enforcement;
  // full access must truthfully report that no confinement was applied.
  const probe = await ctx.shell.execute(ctx.shell.resolve({
    command: ':', workdir: cwd, timeoutMs: 5000,
    sandboxPolicy: { mode: permissionMode, workspaceRoot: cwd },
  }));
  const result = await probe.result();
  if (result.exitCode !== 0 || result.timedOut || result.aborted ||
      result.sandbox?.mode !== permissionMode ||
      (fullAccess ? result.sandbox.denied !== false || result.sandbox.enforcement !== undefined : result.sandbox.enforcement !== 'full')) {
    throw new Error(fullAccess ? 'Flywheel readiness probe did not confirm explicit full access' : 'Flywheel sandbox readiness probe did not prove full enforcement');
  }
  // Session creation is read-only in rc.1. Agent creation is awaited after
  // that announcement and before the factory admits any model work in both hosts.
  ctx.on('agent/created', async ({ agent }) => {
    const session = agent.session;
    if (String(session.id) !== sessionId) throw new Error('Flywheel policy refuses an unrelated SDK session');
    // Official events remain readable by a later SDK. An ad-hoc Session event
    // would make rc.1 reject the entire persisted log on resume.
    session.append('sandbox/mode', { mode: permissionMode });
    session.append('approval/policy', { policy: 'never' });
    const policy = ctx.sandboxPolicy.resolve({ session });
    const receipt = {
      protocol: 1,
      requestId, sessionId, nonce,
      cwd: policy.workspaceRoot,
      permissionMode: policy.mode,
      // Read the explicit logged override through the public API.
      approvalPolicy: ctx.approval.overrideOf(session),
      confinedShell: !fullAccess,
      ...(fullAccess ? {} : { enforcement: result.sandbox.enforcement }),
    };
    // The awaited creation hook publishes once into an owned 0700 directory.
    // Exclusive creation + atomic link rejects existing files/symlinks. This
    // transient control receipt is read before model dispatch, never on replay.
    const pending = receiptPath + '.pending';
    await writeFile(pending, JSON.stringify(receipt), { mode: 0o600, flag: 'wx' });
    await link(pending, receiptPath);
    await unlink(pending);
  });
}
