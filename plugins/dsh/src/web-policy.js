// Read-only admission against the actual SDK services, never a second plugin copy.
import { BridgeError, isChatGPTWeb, record } from './protocol.js';

export async function admitChatGPTWeb(ctx, request, signal = AbortSignal.timeout(10000)) {
  if (!isChatGPTWeb(request) || request.reasoningEffort !== 'pro') {
    throw new BridgeError('INVALID_REQUEST', 'Web admission requires the exact chatgpt/gpt-6-pro-web/pro binding');
  }
  const llm = ctx.get('llm');
  const config = ctx.get('compaction')?.config;
  if (typeof llm?.resolveModelInfo !== 'function' || !record(config) || config.auto !== true ||
      !Array.isArray(config.modelPolicies) || config.modelPolicies.some(value => !record(value) ||
        [value.provider, value.model].some(id => typeof id !== 'string' || id.trim() === ''))) {
    throw new BridgeError('INVALID_REQUEST', 'ChatGPT Web requires the public LLM and configured basic compaction services in this SDK profile');
  }
  const matches = config.modelPolicies.filter(value => value.provider === request.provider && value.model === request.model);
  if (matches.length > 1) throw new BridgeError('INVALID_REQUEST', 'The SDK has duplicate compaction policies for the Web model');
  const override = matches[0];
  const summary = { provider: override?.summarizationProvider ?? config.summarizationProvider,
    model: override?.summarizationModel ?? config.summarizationModel };
  if ([summary.provider, summary.model].some(value => typeof value !== 'string' || value.trim() === '') || summary.model === 'gpt-6-pro-web') {
    throw new BridgeError('INVALID_REQUEST', 'Configure an explicit supported summarizationProvider and summarizationModel in this SDK profile; Web cannot consume summary maxTokens');
  }
  const auxiliary = await llm.resolveModelInfo(summary.provider, summary.model, signal);
  if (auxiliary?.provider !== summary.provider || auxiliary?.id !== summary.model) {
    throw new BridgeError('INVALID_REQUEST', 'The auxiliary provider did not resolve the configured summary model');
  }
  // This is the loaded adapter's live readiness check. A static catalog or a
  // package descriptor cannot prove the current bridge/account binding.
  const info = await llm.resolveModelInfo(request.provider, request.model, signal);
  if (info?.provider !== request.provider || info?.id !== request.model || info.defaultMaxTokens !== undefined ||
      info.reasoning?.defaultEffort !== 'pro' || info.reasoning.efforts?.length !== 1 || info.reasoning.efforts[0]?.id !== 'pro') {
    throw new BridgeError('INVALID_REQUEST', 'Upgrade and configure the ChatGPT plugin with verified local Web bridge readiness and exact pro metadata');
  }
  signal.throwIfAborted();
  return { version: 1, provider: request.provider, model: request.model, reasoningEffort: 'pro',
    ready: true, usage: 'unknown', idle: 'wall-only', summary, title: 'deterministic' };
}

export function verifyChatGPTWebAdmission(receipt, request) {
  if (!isChatGPTWeb(request)) return;
  const web = receipt?.web;
  if (!record(web) || web.version !== 1 || web.provider !== request.provider || web.model !== request.model ||
      web.reasoningEffort !== request.reasoningEffort || web.ready !== true || web.usage !== 'unknown' ||
      web.idle !== 'wall-only' || web.title !== 'deterministic' || !record(web.summary) ||
      [web.summary.provider, web.summary.model].some(value => typeof value !== 'string' || value.trim() === '') || web.summary.model === 'gpt-6-pro-web') {
    throw new BridgeError('INVALID_REQUEST', 'The SDK did not attest verified Web readiness and the explicit auxiliary route; prompt withheld');
  }
}
