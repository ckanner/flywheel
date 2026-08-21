import { test } from 'node:test';
import assert from 'node:assert/strict';
import { priceForModel, estimateCostUsd } from '../orchestrator/core/metrics.js';

const PRICING = {
  'deepseek-v4-flash': { input: 0.28, output: 0.42, 'cache-read': 0.028 },
  'kimi-k3': { input: 3.0, output: 15.0, 'cache-read': 0.30 },
};

test('priceForModel: full id, short id, and kimi-prefixed short all resolve (P1-13)', () => {
  assert.equal(priceForModel(PRICING, 'opencode-go/deepseek-v4-flash'), PRICING['deepseek-v4-flash']);
  assert.equal(priceForModel(PRICING, 'deepseek-v4-flash'), PRICING['deepseek-v4-flash']);
  // The bug: kimi-code/k3 shortens to "k3", which never matched "kimi-k3".
  assert.equal(priceForModel(PRICING, 'kimi-code/k3'), PRICING['kimi-k3']);
  assert.equal(priceForModel(PRICING, 'kimi-coding/k3'), PRICING['kimi-k3']);
  assert.equal(priceForModel(PRICING, 'unknown/model-9'), null);
  assert.equal(priceForModel(null, 'kimi-code/k3'), null);
});

test('estimateCostUsd: computes from the per-1M table, null without usage or price', () => {
  const cost = estimateCostUsd({ input: 1_000_000, output: 100_000, cacheRead: 500_000 }, PRICING['kimi-k3']);
  assert.equal(cost, 3.0 + 1.5 + 0.15);
  assert.equal(estimateCostUsd(null, PRICING['kimi-k3']), null);
  assert.equal(estimateCostUsd({ input: 1 }, null), null);
  // Appendix D fallback: missing breakdown fields must not crash the math.
  assert.equal(estimateCostUsd({ input: 1_000_000, output: 1_000_000 }, PRICING['deepseek-v4-flash']), 0.28 + 0.42);
});
