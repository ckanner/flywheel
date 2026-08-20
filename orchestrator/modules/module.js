// Shared context for the plug-in modules (Intake §4.5, Garden §4.6):
// a lightweight sibling of the run-task context — same watchdog settings,
// same credential hygiene, same transport-retry + channel-fallback chain,
// same per-spawn metrics. Budget usage is recorded into state.json when the
// task directory has one (recording is always on, 铁律 9); modules that run
// before a task exists still append metrics.jsonl.

import { join } from 'node:path';
import { appendMetrics, promptHash, estimateCostUsd } from '../core/metrics.js';
import { classifyError, isRetryable, shouldFallback, computeBackoff, extractRetryAfterMs } from '../core/errors.js';
import { nextChannel, modelForChannel, envKeyForChannel } from '../core/fallback.js';
import { stateExists, loadState, saveState, recordUsage } from '../core/state.js';

const defaultSleep = (ms) => new Promise((r) => setTimeout(r, ms));

export function createModuleContext({ taskDir, cwd = process.cwd(), config, drivers, log = () => {}, sleep = defaultSleep }) {
  return {
    taskDir,
    cwd,
    config,
    drivers,
    log,
    sleep,
    metricsFile: join(taskDir, 'metrics.jsonl'),
  };
}

export function moduleWatchdogOpts(config) {
  const w = config.watchdog ?? {};
  return {
    idleTimeoutMs: w['idle-timeout-ms'],
    wallTimeoutMs: w['wall-timeout-ms'],
    killGraceMs: w['kill-grace-ms'],
  };
}

export function moduleChannelApiKey(config, channel) {
  const envKey = envKeyForChannel(config, channel);
  return envKey && process.env[envKey] ? process.env[envKey] : null;
}

export function moduleApiKeysFor(config, channel) {
  const envKey = envKeyForChannel(config, channel);
  if (envKey && process.env[envKey]) return { [envKey]: process.env[envKey] };
  return {};
}

function recordModuleMetrics(mctx, { role, model, harness, channel, prompt, result, errorCode }) {
  const usage = result.usage ?? null;
  const shortModel = (model ?? '').split('/').pop();
  const costUsd = result.costUsd ?? estimateCostUsd(usage, mctx.config.pricing?.[shortModel] ?? null);
  appendMetrics(mctx.metricsFile, {
    role,
    'packet-id': null,
    milestone: null,
    model: model ?? null,
    harness: harness ?? result.harness ?? null,
    channel: channel ?? null,
    'prompt-hash': prompt ? promptHash(prompt) : null,
    usage,
    'cache-hit-rate': usage && usage.input > 0 ? Math.round(((usage.cacheRead ?? 0) / usage.input) * 1000) / 1000 : null,
    'reasoning-chars': result.reasoningChars ?? null,
    'cost-usd': costUsd,
    'duration-ms': result.durationMs ?? null,
    'exit-code': result.exitCode ?? null,
    'error-code': errorCode ?? null,
    attempt: null,
  });
  // Token/cost recording never turns off — land it in state.json when present.
  if (usage && stateExists(mctx.taskDir)) {
    try {
      const state = loadState(mctx.taskDir);
      recordUsage(state, usage, { provider: result.provider ?? channel ?? 'unknown', costUsd });
      saveState(mctx.taskDir, state, { expectedRevision: state.revision });
    } catch (err) {
      mctx.log(`budget recording skipped: ${err.message}`);
    }
  }
}

/**
 * One agent call with the standard transport-retry + channel-fallback chain
 * (§6). Returns the assistant text on success. Verdict parsing belongs to the
 * caller (each module has its own schema).
 *
 * @returns {Promise<{ok: boolean, text: string, errorCode: string|null, channel: string, model: string}>}
 */
export async function callAgent(mctx, { role, binding, prompt, sessionId = null }) {
  let channel = binding.channel;
  let currentBinding = binding;
  const visitedChannels = new Set();
  const maxTransport = mctx.config.retry?.['max-transport-retries'] ?? 5;
  let lastCode = 'TRANSPORT';

  for (let t = 0; t <= maxTransport; t++) {
    visitedChannels.add(channel);
    const result = await mctx.drivers[role][role]({
      taskDir: mctx.taskDir,
      cwd: mctx.cwd,
      prompt,
      sessionId,
      model: currentBinding.model,
      binding: currentBinding,
      channel,
      apiKey: moduleChannelApiKey(mctx.config, channel),
      apiKeys: moduleApiKeysFor(mctx.config, channel),
      watchdogOpts: moduleWatchdogOpts(mctx.config),
    });
    const code = result.errorMessage
      ? classifyError({ exitCode: 1, stderr: result.errorMessage })
      : result.exitCode === 0 && !result.timedOut && !result.spawnError
        ? null
        : classifyError(result) ?? 'TRANSPORT';
    lastCode = code ?? lastCode;
    recordModuleMetrics(mctx, { role, model: currentBinding.model, harness: result.harness, channel, prompt, result, errorCode: code });

    if (!code) {
      return { ok: true, text: result.text ?? result.stdout ?? '', errorCode: null, channel, model: currentBinding.model };
    }

    mctx.log(`${role} transport ${t}: ${code}`);

    if (shouldFallback(code)) {
      const next = nextChannel(mctx.config, channel);
      if (next && !visitedChannels.has(next)) {
        mctx.log(`${role} fallback: ${channel} -> ${next} (${code})`);
        const mapped = modelForChannel(mctx.config, next, role);
        channel = next;
        currentBinding = { ...binding, model: mapped.model, harness: mapped.harness ?? binding.harness };
        t -= 1; // fallback does not consume a transport retry
        continue;
      }
    }
    if (!isRetryable(code)) {
      return { ok: false, text: '', errorCode: code, channel, model: currentBinding.model };
    }
    const delay = computeBackoff(t, {
      baseDelayMs: mctx.config.retry?.['base-delay-ms'],
      maxDelayMs: mctx.config.retry?.['max-delay-ms'],
      jitter: mctx.config.retry?.jitter,
      retryAfterMs: extractRetryAfterMs(`${result.stderr}\n${result.stdout}`),
    });
    await mctx.sleep(delay);
  }
  return { ok: false, text: '', errorCode: lastCode, channel, model: currentBinding.model };
}
