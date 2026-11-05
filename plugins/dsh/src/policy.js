// Loaded only in the independently owned SDK process, never in the desktop host.
// Public rc.5 Cordis services; the worker verifies this acknowledgement before prompt().
export const inject = ['sandboxPolicy', 'approval', 'shell'];

export async function apply(ctx, config) {
  const { permissionMode, cwd } = config;
  const fullAccess = permissionMode === 'danger-full-access';
  if (!['read-only', 'workspace-write', 'danger-full-access'].includes(permissionMode) || typeof cwd !== 'string' ||
      ctx.shell.sandboxMode === undefined) {
    throw new Error('Flywheel requires a sandbox-capable DSH shell and explicit permission policy');
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
  ctx.on('session/created', session => {
    // A fresh SDK session has no inherited permission facts. Pin both canonical
    // knobs, rather than relying on saved UI preferences or a prompt instruction.
    session.append('sandbox/mode', { mode: permissionMode });
    session.append('approval/policy', { policy: 'never' });
    const policy = ctx.sandboxPolicy.resolve({ session });
    session.append('flywheel/policy', {
      protocol: 1,
      cwd: policy.workspaceRoot,
      permissionMode: policy.mode,
      approvalPolicy: ctx.approval.effectivePolicy(session),
      confinedShell: !fullAccess,
      ...(fullAccess ? {} : { enforcement: result.sandbox.enforcement }),
    });
  });
}
