"use strict";

const assert = require('node:assert/strict');

// Candidate and public-registry installs enforce the same release expectation.
// Keeping it here makes an outdated smoke expectation fail before publication.
function assertSubscriptionCLIProfiles(template) {
  assert.deepEqual(template.subscription, { version: 1, mode: 'subscription-only' });
  assert.equal(template.profiles.hydrogen.harness, 'antigravity');
  assert.equal(template.profiles.hydrogen.model, 'gemini-4-hydrogen-high');
  assert.equal(template.routes.default.profile, 'hydrogen-local');
  assert.equal(template.loops.plan.profile, 'neon');
  assert.equal(template.loops.investigate.profile, 'neon');
  assert.equal(template.routes.backend.profile, 'sol');
  assert.equal(template.routes.complex.profile, 'orchestrator-neon');
  assert.equal(template.routes.multimodal.profile, 'neon');
  assert.equal(template.routes.batch.profile, 'kimi');
  assert.equal(template.profiles.kimi['native-profile'], 'kimi-subscription-agent');
  assert.equal(template['native-profiles']['kimi-subscription-agent'].options.mode, 'agent');
  assert.equal(template['native-profiles']['kimi-subscription-agent'].options.agents.enabled, true);
  assert.deepEqual(template.planning.compact, { 'design-plan': 'hydrogen-local', 'design-review': 'sol-plan' });
  assert.equal(template.planning['dual-draft'].enabled, true);
  assert.equal(template['frontend-ux-review'].enabled, true);
  assert.equal(template.budget.enforce, 'off');
  assert.equal(template.guardrails['max-packet-attempts'], 3);
  assert.equal(template.guardrails['max-review-rounds'], 2);
  assert.equal(template.guardrails['max-task-calls'], 60);
  assert.equal(template.delivery.endpoint, 'local');
  assert.equal(template.jev.enabled, false);
  assert.equal(template.jev.backend, 'subscription');
  for (const channel of Object.values(template.channels)) {
    if (!channel || typeof channel !== 'object') continue;
    assert.equal(channel['env-key'] ?? null, null);
    assert.ok(!channel.fallback);
  }
}

module.exports = { assertSubscriptionCLIProfiles };
