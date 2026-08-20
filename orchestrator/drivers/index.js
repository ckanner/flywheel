// Driver factory: config.drivers picks the implementation per role
// (mock for tests/dry runs, pi for Execute/Review/Judge, kimi for Plan).
// The binding's harness (from the §13 binding table, possibly overridden by a
// fallback channel) decides between pi and kimi on every call.

import { createMockDriver } from './mock.js';
import { runPi } from './pi.js';
import { runKimi, KIMI_DEFAULT_MODEL } from './kimi.js';

export function createDrivers(config) {
  const sel = config.drivers ?? {};
  const mock = createMockDriver(config['mock-behaviors']);

  // Generic harness dispatch shared by all roles: harness comes from the
  // (possibly fallback-rewritten) binding, never from the role.
  const invokeHarness = (args) => {
    const harness = args.binding?.harness ?? 'pi';
    if (harness === 'kimi') {
      return runKimi({
        prompt: args.prompt,
        sessionId: args.sessionId ?? null,
        model: args.model ?? args.binding?.model ?? KIMI_DEFAULT_MODEL,
        cwd: args.cwd,
        apiKeys: args.apiKeys,
        watchdogOpts: args.watchdogOpts,
      });
    }
    return runPi({
      prompt: args.prompt,
      model: args.model ?? args.binding?.model,
      thinking: args.binding?.thinking ?? 'max',
      cwd: args.cwd,
      apiKey: args.apiKey ?? null,
      apiKeys: args.apiKeys,
      watchdogOpts: args.watchdogOpts,
      onEvent: args.onEvent,
    });
  };

  const real = { name: 'real', execute: invokeHarness, review: invokeHarness, judge: invokeHarness, plan: invokeHarness, intake: invokeHarness, garden: invokeHarness };

  return {
    plan: sel.plan === 'mock' ? mock : { name: 'kimi', plan: real.plan },
    execute: sel.execute === 'mock' ? mock : { name: 'real', execute: real.execute },
    review: sel.review === 'mock' ? mock : { name: 'real', review: real.review },
    judge: sel.judge === 'mock' ? mock : { name: 'real', judge: real.judge },
    intake: sel.intake === 'mock' ? mock : { name: 'real', intake: real.intake },
    garden: sel.garden === 'mock' ? mock : { name: 'real', garden: real.garden },
  };
}
