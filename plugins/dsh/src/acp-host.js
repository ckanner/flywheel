const commandActions = new Set(['doctor', 'connect', 'status', 'list', 'cancel']);

export function acpCommandSummary(value) {
  if (!value || value.protocol !== 2) throw new Error('The ACP Host returned an invalid control response.');
  const result = { protocol: 2 };
  for (const key of ['id', 'transport', 'sourceCwd', 'cwd', 'branch', 'baseCommit', 'status', 'connection', 'revision', 'createdAt', 'updatedAt']) {
    if (['string', 'number', 'boolean'].includes(typeof value[key])) result[key] = value[key];
  }
  if (value.runtime) result.runtime = Object.fromEntries(['executable', 'version', 'sha256', 'platform', 'architecture']
    .filter(key => typeof value.runtime[key] === 'string').map(key => [key, value.runtime[key]]));
  if (value.knownWork) result.knownWork = Object.fromEntries(['tools', 'permissions', 'children', 'unknown']
    .filter(key => ['number', 'boolean'].includes(typeof value.knownWork[key])).map(key => [key, value.knownWork[key]]));
  if (value.cleanup) result.cleanup = Object.fromEntries(['process', 'session']
    .filter(key => typeof value.cleanup[key] === 'string').map(key => [key, value.cleanup[key]]));
  if (Array.isArray(value.sessions)) result.sessions = value.sessions.map(item => acpCommandSummary({ ...item, protocol: 2 }));
  result.view = 'Open the Antigravity view for prompts, permissions, sign-in and full session details.';
  return result;
}

// Keep the optional Host imports behind the public Typert composition. The Go
// workers and CLI-only package imports must not boot another interactive agent.
export function installFlywheelAcp(ctx, getConfig, { loadHost = () => import('@ckanner/dsh-subagent-antigravity/acp/host') } = {}) {
  if (typeof ctx.inject !== 'function') return null;
  let service, disposed = false;
  ctx.effect(() => () => { disposed = true; }, 'flywheel: ACP command lifetime');
  ctx.commands.register({
    name: 'flywheel-antigravity',
    description: 'Open or inspect an official Antigravity ACP interactive session. Prompts and permissions belong to the Antigravity view.',
    recordInput: false,
    input: { hint: 'Leave empty to open the Antigravity entry. Optional controls: {"version":2,"action":"connect"}, doctor, list, status, cancel. Send prompts in the view.' },
    async handler({ rawInput, agent, signal }) {
      try {
        if (!String(rawInput ?? '').trim()) {
          if (disposed) throw new Error('The Antigravity ACP Host is unavailable.');
          return { kind: 'success', text: 'Open the Antigravity view, check the official CLI and start a new isolated session. Complete native sign-in there if needed. Prompts, code and permission responses belong to that authenticated view; this command has not started an agent or sent a task.' };
        }
        let input;
        try { input = JSON.parse(rawInput); }
        catch { throw new Error('Enter a version 2 JSON control action; send prompts and code in the Antigravity view.'); }
        if (!input || typeof input !== 'object' || Array.isArray(input) || input.version !== 2 || !commandActions.has(input.action) ||
            Object.keys(input).some(key => !['version', 'action', ...(['status', 'cancel'].includes(input.action) ? ['id'] : [])].includes(key)) ||
            (input.action === 'cancel' || 'id' in input) && (typeof input.id !== 'string' || !input.id || input.id.length > 128 || /[\x00-\x1f\x7f]/.test(input.id))) {
          throw new Error('Use doctor, connect, list, status or cancel here. Prompts, code, authentication and permission responses must use the Antigravity view.');
        }
        if (disposed || !service) throw new Error('The Antigravity ACP Host is unavailable or still initializing. Open the Antigravity view after the Host is ready.');
        let value;
        try { value = await service.action(agent, input, signal); }
        catch { throw new Error('The ACP control request was not confirmed. Open the Antigravity view to inspect the original session and its diagnostic; no prompt was replayed.'); }
        return { kind: 'success', text: JSON.stringify(acpCommandSummary(value)) };
      } catch (error) {
        return { kind: 'error', text: error instanceof Error ? error.message : String(error) };
      }
    },
  });
  const ready = ctx.inject(['typert'], async scope => {
    const { installAcpHost } = await loadHost();
    if (typeof installAcpHost !== 'function') throw new Error('The bundled Antigravity plugin does not provide its ACP Host entry. Reinstall the complete Flywheel package.');
    service = await installAcpHost(scope, getConfig);
    if (!service || typeof service.action !== 'function') throw new Error('The Antigravity ACP Host did not install its interactive service.');
  });
  return { ready };
}

let workspaceApi;
export async function protectAcpWorkspace(cwd) {
  // An absent/invalid cwd remains a normal pre-dispatch validation error. Never
  // guess process.cwd() or substitute a different project for the conversation.
  if (cwd == null) throw new Error('The current conversation has no working directory (cwd).');
  workspaceApi ??= import('@ckanner/dsh-subagent-antigravity/acp/workspace');
  const { assertNotOwnedAcpWorkspace } = await workspaceApi;
  if (typeof assertNotOwnedAcpWorkspace !== 'function') throw new Error('The bundled Antigravity workspace guard is unavailable. Reinstall the complete Flywheel package.');
  const physical = await realpath(cwd);
  await assertNotOwnedAcpWorkspace(physical);
  return physical;
}
import { realpath } from 'node:fs/promises';
