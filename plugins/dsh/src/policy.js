// Loaded only in the independently owned SDK process, never in the desktop host.
// Public rc.5/rc.1 Cordis services; the worker verifies this acknowledgement before prompt().
import { link, realpath, stat, unlink, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute } from 'node:path';
import { readPolicyReceipt, nonempty } from './protocol.js';
import { createNativePolicy } from './native-policy.js';
import { createDshActivityPublisher } from './dsh-activity.js';
import { createWebPolicy } from './web-policy.js';

export const inject = ['sandboxPolicy', 'approval', 'shell', 'workingDirectory'];

export async function apply(ctx, config) {
  const { permissionMode, cwd, workspaceCwd, receiptPath, requestId, nonce, deferredAdmissionPath } = config;
  let { sessionTitle, sessionId } = config;
  let hostInvocationId = config.hostInvocationId, expectedBinding = config.web?.expectedBinding, deadlineMs = config.deadlineMs;
  let nativePolicy;
  const fullAccess = permissionMode === 'danger-full-access';
  if (!['read-only', 'workspace-write', 'danger-full-access'].includes(permissionMode) || typeof cwd !== 'string' ||
      ctx.shell.sandboxMode === undefined) {
    throw new Error('Flywheel requires a sandbox-capable DSH shell and explicit permission policy');
  }
  if (typeof receiptPath !== 'string' || !isAbsolute(receiptPath) ||
      [requestId, ...(deferredAdmissionPath === undefined ? [sessionId] : []), nonce].some(value => typeof value !== 'string' || value.trim() === '' || value.includes('\0'))) {
    throw new Error('Flywheel policy requires an owned, correlated admission receipt');
  }
  if (deferredAdmissionPath !== undefined && (typeof deferredAdmissionPath !== 'string' || !isAbsolute(deferredAdmissionPath) ||
      dirname(deferredAdmissionPath) !== dirname(receiptPath) || sessionId !== undefined)) {
    throw new Error('Flywheel deferred admission requires the same owned receipt directory');
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
  const webPolicy = await createWebPolicy(ctx, config);
  const activity = config.activity?.version === 1 ? createDshActivityPublisher(ctx, {
    identity: () => ({ requestId, nonce, rootSessionId: sessionId }),
    idleTimeoutMs: config.activity.idleTimeoutMs,
    children: config.native?.agents?.enabled === true,
    ...(config.web ? { webIdentity: () => ({ hostInvocationId, executionDomainId: config.web.executionDomainId }) } : {}),
  }) : null;
  // Session creation is read-only in rc.1. Agent creation is awaited after
  // that announcement and before the factory admits any model work in both hosts.
  ctx.on('agent/created', async ({ agent }) => {
    const session = agent.session;
    if (deferredAdmissionPath !== undefined) {
      const identity = await readPolicyReceipt(deferredAdmissionPath, { requestId, nonce });
      if (Object.keys(identity).some(key => !['requestId', 'nonce', 'sessionId', 'sessionTitle', 'managed', 'resuming', 'hostInvocationId', 'expectedWebBinding', 'deadlineMs'].includes(key))) {
        throw new Error('Flywheel deferred admission has unsupported fields');
      }
      sessionId = nonempty(identity.sessionId, 'sessionId', 512);
      sessionTitle = identity.sessionTitle === undefined ? undefined : nonempty(identity.sessionTitle, 'sessionTitle', 512);
      config.managed = identity.managed;
      config.resuming = identity.resuming;
      hostInvocationId = identity.hostInvocationId;
      expectedBinding = identity.expectedWebBinding;
      deadlineMs = identity.deadlineMs;
    }
    config.sessionId = sessionId;
    nativePolicy ??= await createNativePolicy(ctx, config);
    const root = String(session.id) === sessionId;
    if (!root && !nativePolicy?.isChild(agent)) throw new Error('Flywheel policy refuses an unrelated SDK session');
    if (workspaceCwd !== undefined && session.header.cwd !== workspaceCwd) {
      throw new Error('Flywheel refuses to relabel a session from a different project workspace');
    }
    if (workspaceCwd !== undefined) {
      // Public service accepts an unpublished Agent. Switch before attesting
      // readiness. The native sandbox root remains the immutable project;
      // unsupported split workspace-write requests are rejected at admission.
      await ctx.workingDirectory.set(agent, cwd);
    }
    if (root && sessionTitle) {
      // The public title service owns UTF-8 normalization and its configured
      // byte limit. An extra character slice discards useful task/scope names.
      const titles = ctx.get('sessionTitle');
      if (typeof titles?.rename !== 'function' || typeof titles?.get !== 'function') {
        throw new Error('Flywheel requires the public sessionTitle service to name a new Session');
      }
      const accepted = titles.rename(session, sessionTitle);
      if (typeof accepted?.title !== 'string' || accepted.title.trim() === '' || titles.get(session)?.title !== accepted.title) {
        throw new Error('Flywheel could not verify the requested Session title');
      }
    }
    // Official events remain readable by a later SDK. An ad-hoc Session event
    // would make rc.1 reject the entire persisted log on resume.
    session.append('sandbox/mode', { mode: permissionMode });
    session.append('approval/policy', { policy: 'never' });
    const native = await nativePolicy?.admit(agent, root);
    activity?.admit(agent, root);
    if (!root) return;
    const web = await webPolicy?.admit(agent, hostInvocationId, expectedBinding, deadlineMs);
    const policy = ctx.sandboxPolicy.resolve({ session });
    const receipt = {
      protocol: 1,
      requestId, sessionId, nonce,
      ...(native ? { native } : {}),
      ...(web ? { web } : {}),
      ...(activity ? { activity: activity.capabilities } : {}),
      ...(workspaceCwd === undefined ? {} : { workspaceCwd: session.header.cwd }),
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
