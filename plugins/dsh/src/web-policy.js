// Public ChatGPT plugin host contract. No DOM, credentials or summaries live here.
import { open, link, unlink } from 'node:fs/promises';
import { dirname, join, isAbsolute } from 'node:path';
import { BridgeError, record } from './protocol.js';

export const WEB_MODEL = 'gpt-6-pro-web';
const text = value => typeof value === 'string' && value.trim() !== '' && value.length <= 512 && !/[\0\r\n]/.test(value);
const keys = (value, allowed) => record(value) && Object.keys(value).every(key => allowed.includes(key));
const bindingKeys = ['protocol','transport','provider','model','reasoningEffort','connectionId','accountFingerprint','policyVersion','contextPolicy','observation','usage','hostContractVersion','runtimeVersion'];
export function validateWebBinding(value, request, expected = request.web?.expectedBinding) {
  if (!keys(value, bindingKeys) || value.protocol !== 1 || value.transport !== 'chatgpt-web' || value.provider !== request.provider ||
      value.model !== WEB_MODEL || value.model !== request.model || value.reasoningEffort !== 'pro' || value.reasoningEffort !== request.reasoningEffort ||
      value.contextPolicy !== 'project-only' || value.observation !== 'opaque' || value.usage !== 'unknown' || value.hostContractVersion !== 1 ||
      value.runtimeVersion !== 'web-runtime/1' || !['connectionId','accountFingerprint','policyVersion'].every(key => text(value[key]))) {
    throw new BridgeError('INVALID_REQUEST', 'Web Pro requires the matching isolated model, account, connection and public host contract');
  }
  if (expected && bindingKeys.some(key => value[key] !== expected[key])) throw new BridgeError('INVALID_REQUEST', 'Web binding changed since the admitted history or prewarm');
  return Object.fromEntries(bindingKeys.map(key => [key, value[key]]));
}
export function validateWebRequest(request) {
  if (request.model !== WEB_MODEL) {
    if (request.web !== undefined) throw new BridgeError('INVALID_REQUEST', 'Web extension requires the exact Web model');
    return;
  }
  const web = request.web;
  if (!keys(web, ['version','executionDomainId','requireIsolation','expectedBinding','probe','recoveryDirectory']) || web.version !== 1 || !text(web.executionDomainId) ||
      web.requireIsolation !== true || web.probe !== undefined && web.probe !== true || request.reasoningEffort !== 'pro' ||
      web.recoveryDirectory !== undefined && (typeof web.recoveryDirectory !== 'string' || !isAbsolute(web.recoveryDirectory) || web.recoveryDirectory.includes('\0'))) {
    throw new BridgeError('INVALID_REQUEST', 'Web Pro requires extension 1, a persistent execution domain, project-only isolation and explicit pro effort');
  }
  if (web.expectedBinding !== undefined) validateWebBinding(web.expectedBinding, request);
}
export function validateWebFacts(value, identity) {
  const fields = ['version','hostInvocationId','executionDomainId','sessionId','dispatch','outcome','remoteGeneration','localCleanupConfirmed','usageStatus'];
  if (!record(value) || value.version !== 1 || ['hostInvocationId','executionDomainId','sessionId'].some(key => !text(value[key]) || value[key] !== identity[key]) ||
      !['not-dispatched','dispatched','unknown'].includes(value.dispatch) || !['completed','failed','cancelled','unknown'].includes(value.outcome) ||
      !['idle','running','unknown'].includes(value.remoteGeneration) || typeof value.localCleanupConfirmed !== 'boolean' || value.usageStatus !== 'unknown' ||
      value.dispatch === 'unknown' && value.outcome !== 'unknown' || value.dispatch === 'not-dispatched' && value.outcome === 'completed' ||
      value.remoteGeneration !== 'idle' && value.outcome !== 'unknown') {
    throw new BridgeError('TRANSPORT', 'Web invocation facts are incomplete, contradictory or belong to another invocation');
  }
  // Plugin-owned detailed invocation journals are not copied into the worker frame.
  return Object.fromEntries(fields.map(key => [key, value[key]]));
}
async function publish(path, value) {
  const pending = path + '.pending';
  const file = await open(pending, 'wx', 0o600);
  try { await file.writeFile(JSON.stringify(value)); await file.sync(); }
  finally { await file.close(); }
  await link(pending, path);
  await unlink(pending);
  const directory = await open(dirname(path), 'r');
  try { await directory.sync(); } finally { await directory.close(); }
}
export async function createWebPolicy(ctx, config) {
  if (config.web === undefined) return null;
  validateWebRequest(config);
  const host = ctx.get('chatgptWebHost');
  if (host?.version !== 1 || !['admitBinding','beginInvocation','invocationFacts'].every(key => typeof host[key] === 'function')) {
    throw new BridgeError('INVALID_REQUEST', 'Install the ChatGPT Web plugin and its compatible public host contract in this SDK profile');
  }
  const compaction = ctx.get('compaction');
  if (!compaction || typeof compaction.compactIfNeeded !== 'function') throw new BridgeError('INVALID_REQUEST', 'Web Pro requires the profile\'s single public CompactionEngine');
  const initial = validateWebBinding(await host.admitBinding({ provider: config.provider, model: config.model,
    reasoningEffort: config.reasoningEffort, requireIsolation: true, executionDomainId: config.web.executionDomainId, purpose: 'preflight' }), config);
  const root = dirname(config.receiptPath);
  await publish(join(root, 'web-admission.json'), { requestId: config.requestId, nonce: config.nonce, binding: initial });
  let identity;
  ctx.effect(() => async () => {
    if (!identity) return;
    let terminal;
    try {
      const facts = validateWebFacts(await host.invocationFacts(identity), identity);
      terminal = { version: 1, binding: initial, facts };
    } catch (error) { terminal = { error: error instanceof Error ? error.message : String(error) }; }
    await publish(join(root, 'web-terminal.json'), { requestId: config.requestId, nonce: config.nonce, ...terminal });
  });
  return {
    binding: initial,
    async admit(agent, hostInvocationId, expectedBinding, deadlineMs) {
      const current = { sessionId: String(agent.session.id), executionDomainId: config.web.executionDomainId, hostInvocationId };
      if (!text(hostInvocationId)) throw new BridgeError('INVALID_REQUEST', 'Web host invocation identity is missing');
      await host.beginInvocation({ ...current, workspace: config.workspaceCwd ?? config.cwd, requireIsolation: true, deadlineMs });
      identity = current; // Failed later admission still needs this invocation's cleanup facts.
      const actual = await host.admitBinding({ ...current, provider: config.provider, model: config.model,
        reasoningEffort: config.reasoningEffort, requireIsolation: true, deadlineMs });
      validateWebBinding(actual, config, initial);
      if (expectedBinding) validateWebBinding(actual, config, expectedBinding);
      if (typeof host.recoveryCheckpoint === 'function') {
        const checkpoint = await host.recoveryCheckpoint(current);
        await publish(join(root, 'web-checkpoint.json'), { requestId: config.requestId, nonce: config.nonce, checkpoint });
      }
      return initial;
    },
  };
}
