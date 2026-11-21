// Subscription admission uses native read-only observations, never credential files.
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const present = value => value !== undefined && value !== null && value !== '';

export class SubscriptionError extends Error {
  constructor(code, message) { super(message); this.name = 'SubscriptionError'; this.code = code; }
}

export function validateSubscriptionPolicy(policy) {
  if (policy === undefined) return false;
  if (!record(policy) || policy.version !== 1 || policy.mode !== 'subscription-only' || Object.keys(policy).length !== 2) {
    throw new SubscriptionError('INVALID_REQUEST', 'Expected subscription version 1 with mode subscription-only');
  }
  return true;
}

// Go supplies a complete, allowlisted child environment. Direct worker callers
// receive the same fail-closed treatment instead of silently dropping conflicts.
export function inspectSubscriptionEnvironment(env, harness) {
  const patterns = harness === 'codex-pro'
    ? /^(?:OPENAI_(?:API_KEY|BASE_URL|API_BASE|ORGANIZATION|PROJECT)|CODEX_(?:API_KEY|BASE_URL|OSS_BASE_URL|AUTH_TOKEN|ACCESS_TOKEN)|AZURE_OPENAI_.*|AWS_(?:ACCESS_KEY_ID|SECRET_ACCESS_KEY|SESSION_TOKEN|PROFILE|BEARER_TOKEN_BEDROCK)|GOOGLE_APPLICATION_CREDENTIALS|ANTHROPIC_API_KEY)$/
    : /^(?:KIMI_(?:API_KEY|BASE_URL|CODE_BASE_URL|CODE_OAUTH_HOST|OAUTH_HOST|CODE_CUSTOM_HEADERS|WEB_(?:SEARCH|FETCH)_(?:API_KEY|BASE_URL))|MOONSHOT_API_KEY|OPENAI_(?:API_KEY|BASE_URL)|ANTHROPIC_(?:API_KEY|AUTH_TOKEN|BASE_URL|CUSTOM_HEADERS)|GOOGLE_(?:API_KEY|APPLICATION_CREDENTIALS)|GEMINI_API_KEY|AWS_(?:ACCESS_KEY_ID|SECRET_ACCESS_KEY|SESSION_TOKEN|PROFILE|BEARER_TOKEN_BEDROCK))$/;
  const conflicts = Object.keys(env).filter(key => patterns.test(key) && present(env[key])).sort();
  if (conflicts.length) throw new SubscriptionError('AUTH', 'Subscription-only conflicts with configured routing or credential environment: ' + conflicts.join(', '));
}

function officialChatGPTURL(value) {
  if (!present(value)) return true;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' && ['chatgpt.com', 'chat.openai.com'].includes(url.hostname) &&
      !url.username && !url.password && !url.port && !url.search && !url.hash && /^\/backend-api(?:\/codex)?\/?$/.test(url.pathname);
  } catch { return false; }
}

// config/read resolves workspace/profile layers in the very app-server that will
// run the task. Null model_provider is the documented built-in OpenAI default.
export function inspectCodexSubscription(account, response) {
  if (!record(account) || account.requiresOpenaiAuth !== true || account.account?.type !== 'chatgpt') {
    throw new SubscriptionError('AUTH', 'Authentication failed: subscription-only requires native Codex Sign in with ChatGPT');
  }
  const config = response?.config;
  if (!record(config) || !Object.hasOwn(config, 'model_provider')) {
    throw new SubscriptionError('INVALID_REQUEST', 'Codex config/read did not expose effective model_provider; subscription admission is unavailable');
  }
  if (config.model_provider !== null && config.model_provider !== 'openai') {
    throw new SubscriptionError('AUTH', 'Subscription-only requires the built-in openai provider; custom, API and cloud providers are not admitted');
  }
  if (!officialChatGPTURL(config.chatgpt_base_url) || (present(config.forced_login_method) && config.forced_login_method !== 'chatgpt')) {
    throw new SubscriptionError('AUTH', 'Subscription-only conflicts with the native ChatGPT route or login policy');
  }
  // Native profiles can load separate provider configuration for child agents.
  // Those files are not part of config/read's resolved root snapshot; do not
  // read them (they may hold credentials) or pretend the root proves them.
  const examine = (value, path = '') => {
    if (!record(value)) return;
    for (const [key, child] of Object.entries(value)) {
      if ((key === 'model_provider' && present(child) && child !== 'openai') ||
          (path.startsWith('agents') && key === 'config_file' && present(child))) {
        throw new SubscriptionError('AUTH', 'Subscription-only cannot admit a native child provider override or separate agent config_file');
      }
      if (record(child)) examine(child, path ? path + '.' + key : key);
    }
  };
  // Inactive named profiles/providers may legitimately exist. Only the selected
  // effective root and child-agent routes are relevant, not an unused catalog.
  examine(config.agents, 'agents');
  const openai = config.model_providers?.openai;
  if (record(openai) && Object.entries(openai).some(([key, value]) =>
      ['base_url', 'env_key', 'experimental_bearer_token', 'auth', 'http_headers', 'env_http_headers'].includes(key) && present(value))) {
    throw new SubscriptionError('AUTH', 'Subscription-only refuses configured OpenAI routing, header or credential overrides');
  }
  return { version: 1, mode: 'subscription-only', status: 'verified-native-controls',
    provider: 'openai', authentication: 'chatgpt', entitlement: 'unknown', quota: 'unknown', extraUsage: 'unknown' };
}

export const KIMI_SUBSCRIPTION_DISABLED_TOOLS = Object.freeze(['WebSearch', 'FetchURL']);
const kimiBases = new Set(['https://api.kimi.com/coding/v1', 'https://api.kimi.ai/coding/v1']);
const kimiEndpoint = value => typeof value === 'string' && kimiBases.has(value.replace(/\/+$/, ''));

export function inspectKimiSubscription(providers, config, model) {
  if (!Array.isArray(providers?.items) || !record(config?.models)) {
    throw new SubscriptionError('INVALID_REQUEST', 'Installed Kimi does not expose the redacted provider and model configuration needed for subscription admission');
  }
  const provider = providers.items.find(item => item?.id === 'managed:kimi-code');
  if (provider?.type !== 'kimi' || provider.status !== 'connected' || provider.has_api_key !== false || present(provider.api_key_env) || !kimiEndpoint(provider.base_url)) {
    throw new SubscriptionError('AUTH', 'Authentication failed: subscription-only requires the connected managed:kimi-code OAuth provider with its official coding endpoint and no API key');
  }
  const aliases = config.models;
  const checkModel = alias => {
    const value = aliases[alias];
    const resolved = record(value?.overrides) ? { ...value, ...value.overrides } : value;
    if (!record(resolved) || (resolved.providerId ?? resolved.provider) !== 'managed:kimi-code' ||
        resolved.has_api_key !== false || present(resolved.apiKeyEnv) || present(resolved.api_key_env) ||
        present(resolved.baseUrl) || present(resolved.base_url) || present(resolved.customHeaders) || present(resolved.custom_headers) ||
        present(resolved.endpoint) || present(resolved.oauth) || present(resolved.apiKey) || present(resolved.api_key)) {
      throw new SubscriptionError('AUTH', 'Subscription-only refuses an unknown model alias or model-level provider, endpoint, header or credential override');
    }
  };
  checkModel(model);
  // Kimi native agents can select other configured aliases. A root OAuth model
  // alone is not sufficient proof for the complete task tree.
  for (const alias of Object.keys(aliases)) checkModel(alias);
  const secondary = config.secondary_model;
  if (present(secondary)) {
    if (!record(secondary) || (present(secondary.defaultModel) && !Object.hasOwn(aliases, secondary.defaultModel)) ||
        (present(secondary.models) && (!record(secondary.models) || Object.keys(secondary.models).some(alias => !Object.hasOwn(aliases, alias))))) {
      throw new SubscriptionError('AUTH', 'Subscription-only cannot verify the configured Kimi secondary model pool');
    }
  }
  return { version: 1, mode: 'subscription-only', status: 'verified-native-controls',
    provider: provider.id, authentication: 'native-oauth', endpoint: provider.base_url.replace(/\/+$/, ''),
    entitlement: 'unknown', quota: 'unknown', extraUsage: 'unknown',
    disabledTools: [...KIMI_SUBSCRIPTION_DISABLED_TOOLS],
    serviceCredentialObservation: 'redacted-ambiguous-disabled-per-prompt' };
}

export async function inspectKimiRuntimeSubscription(runtime, model, signal) {
  const [providers, config] = await Promise.all(['/api/v1/providers', '/api/v1/config'].map(path => runtime.request(path, undefined, { signal, timeout: 5000 })));
  return inspectKimiSubscription(providers, config, model);
}
