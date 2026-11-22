import test from 'node:test';
import assert from 'node:assert/strict';
import { inspectCodexSubscription, inspectKimiSubscription, inspectSubscriptionEnvironment, validateSubscriptionPolicy } from '../src/subscription-controls.js';

const account = { account: { type: 'chatgpt', email: 'private@example.invalid', planType: 'pro' }, requiresOpenaiAuth: true };
const config = { config: { model_provider: 'openai' } };
const providers = { items: [{ id: 'managed:kimi-code', type: 'kimi', status: 'connected', has_api_key: false, base_url: 'https://api.kimi.com/coding/v1' }] };
const kimiConfig = { models: { 'kimi-code/k3.1-pro': { provider: 'managed:kimi-code', model: 'k3.1-pro', has_api_key: false } } };

test('subscription policy is explicit and rejects unknown or malformed controls', () => {
  assert.equal(validateSubscriptionPolicy(undefined), false);
  assert.equal(validateSubscriptionPolicy({ version: 1, mode: 'subscription-only' }), true);
  for (const policy of [null, [], {}, { version: 2, mode: 'subscription-only' }, { version: 1, mode: 'api' }, { version: 1, mode: 'subscription-only', apiFallback: false }]) {
    assert.throws(() => validateSubscriptionPolicy(policy), { code: 'INVALID_REQUEST' });
  }
});

test('native Codex route and auth must both agree without exposing identity or inventing quota', () => {
  const accepted = inspectCodexSubscription(account, config);
  assert.equal(accepted.status, 'verified-native-controls');
  assert.equal(accepted.quota, 'unknown');
  assert.doesNotMatch(JSON.stringify(accepted), /private@example|planType/);
  assert.equal(inspectCodexSubscription(account, { config: { model_provider: null, chatgpt_base_url: 'https://chatgpt.com/backend-api/' } }).authentication, 'chatgpt');
  for (const bad of [{ account: null, requiresOpenaiAuth: true }, { account: { type: 'apiKey' }, requiresOpenaiAuth: true }, { ...account, requiresOpenaiAuth: false }]) {
    assert.throws(() => inspectCodexSubscription(bad, config), { code: 'AUTH' });
  }
  for (const patch of [{ model_provider: 'custom' }, { chatgpt_base_url: 'https://attacker.invalid' }, { chatgpt_base_url: 'http://chatgpt.com/backend-api' },
    { chatgpt_base_url: 'https://user:secret@chatgpt.com/backend-api' }, { forced_login_method: 'api' },
    { model_providers: { openai: { env_key: 'PAY_API_KEY' } } }, { agents: { reviewer: { config_file: 'private.toml' } } },
    { agents: { reviewer: { model_provider: 'api' } } }]) {
    assert.throws(() => inspectCodexSubscription(account, { config: { ...config.config, ...patch } }), { code: 'AUTH' });
  }
  assert.throws(() => inspectCodexSubscription(account, { config: {} }), { code: 'INVALID_REQUEST' });
  // Unselected providers and profiles do not rewrite the selected native route.
  assert.equal(inspectCodexSubscription(account, { config: { ...config.config, model_providers: { unused: { env_key: 'UNUSED' } }, profiles: { api: { model_provider: 'api' } } } }).status, 'verified-native-controls');
});

test('Kimi checks native OAuth, active and secondary model routes; ambiguous service credentials are disabled explicitly', () => {
  const observed = inspectKimiSubscription(providers, kimiConfig, 'kimi-code/k3.1-pro');
  assert.equal(observed.authentication, 'native-oauth');
  assert.deepEqual(observed.disabledTools, ['WebSearch', 'FetchURL']);
  assert.equal(observed.extraUsage, 'unknown');
  for (const patch of [{ has_api_key: true }, { api_key_env: 'BILLING_KEY' }, { status: 'unconfigured' }, { type: 'bedrock' }, { base_url: 'https://api.kimi.com.evil.invalid/coding/v1' }]) {
    assert.throws(() => inspectKimiSubscription({ items: [{ ...providers.items[0], ...patch }] }, kimiConfig, 'kimi-code/k3.1-pro'), { code: 'AUTH' });
  }
  for (const patch of [{ has_api_key: true }, { provider: 'api' }, { baseUrl: 'https://example.invalid' }, { customHeaders: { authorization: 'private' } },
    { overrides: { provider: 'api' } }, { endpoint: {} }]) {
    assert.throws(() => inspectKimiSubscription(providers, { models: { 'kimi-code/k3.1-pro': { ...kimiConfig.models['kimi-code/k3.1-pro'], ...patch } } }, 'kimi-code/k3.1-pro'), { code: 'AUTH' });
  }
  assert.throws(() => inspectKimiSubscription(providers, kimiConfig, 'missing'), { code: 'AUTH' });
  assert.throws(() => inspectKimiSubscription(providers, { ...kimiConfig, secondary_model: { defaultModel: 'api', models: { api: '' } } }, 'kimi-code/k3.1-pro'), { code: 'AUTH' });
  assert.equal(inspectKimiSubscription(providers, { ...kimiConfig, secondary_model: { defaultModel: 'kimi-code/k3.1-pro', models: { 'kimi-code/k3.1-pro': 'coding' } } }, 'kimi-code/k3.1-pro').status, 'verified-native-controls');
  assert.throws(() => inspectKimiSubscription({}, {}, 'missing'), { code: 'INVALID_REQUEST' });
});

test('explicit credential and routing conflicts are refused by name, never silently overwritten or printed', () => {
  for (const [harness, key] of [['codex-pro', 'OPENAI_API_KEY'], ['codex-pro', 'CODEX_BASE_URL'], ['kimi', 'KIMI_CODE_CUSTOM_HEADERS'], ['kimi', 'KIMI_WEB_SEARCH_API_KEY']]) {
    assert.throws(() => inspectSubscriptionEnvironment({ [key]: 'secret-must-not-appear' }, harness), error => error.code === 'AUTH' && error.message.includes(key) && !error.message.includes('secret-must-not-appear'));
  }
  assert.doesNotThrow(() => inspectSubscriptionEnvironment({ HOME: '/isolated', CODEX_HOME: '/native', OPENAI_API_KEY: '' }, 'codex-pro'));
});
