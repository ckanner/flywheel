import { readFileSync } from 'node:fs';
import { runWorker } from '../../src/bridge.js';
import { beginWebRecovery, WEB_RECOVERY_VERSION } from '../../src/web-recovery.js';

const { request, env, unsent } = JSON.parse(readFileSync(0, 'utf8'));
const frames = [];
let result;
if (unsent) {
  const sessionId = 'original-unsent-session';
  const web = { protocol: 1, transport: 'chatgpt-web', provider: request.provider, model: request.model, reasoningEffort: 'pro',
    connectionId: 'offline-connection', accountFingerprint: 'offline-account', policyVersion: 'fixture-policy/1', contextPolicy: 'project-only',
    observation: 'opaque', usage: 'unknown', hostContractVersion: 1, runtimeVersion: 'web-runtime/1' };
  const ready = { type: 'ready', protocol: 2, requestId: request.requestId, sessionId, cwd: request.cwd, provider: request.provider, model: request.model,
    reasoningEffort: 'pro', permissionMode: request.permissionMode, approvalPolicy: 'never', persistent: true, resumed: false, web };
  await beginWebRecovery(request, { version: 1, contract: WEB_RECOVERY_VERSION, ownerPid: process.pid,
    identity: { sessionId, hostInvocationId: request.requestId, executionDomainId: request.web.executionDomainId }, fixture: true }, ready, env);
  frames.push(ready);
} else result = await runWorker(request, { env, emit: frame => frames.push(frame), stderr: () => {} });
process.stdout.write(JSON.stringify({ result, frames }));
