import { createRequire } from 'node:module';
import { isAbsolute } from 'node:path';
import { pathToFileURL } from 'node:url';
import { CatalogProcess, CatalogError } from './model-catalog-process.js';
import { validModelCatalogRequest } from './model-catalog-contract.js';

export const MODEL_CATALOG_LIMIT = 256;
const SOURCES = Object.freeze(Object.assign(Object.create(null), { antigravity: 'agy models', 'codex-pro': 'codex app-server model/list',
  'claude-code-pro': 'Claude Agent SDK initialization.models' }));
const EFFORTS = ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'];
const id = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$/.test(value);
const readable = (value, limit = 256) => typeof value === 'string' && value.trim() !== '' && Buffer.byteLength(value) <= limit && !/[\p{Cc}\p{Cf}]/u.test(value);
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);

export function validateCatalogRequest(value) {
  const binding = record(value) ? Object.fromEntries(Object.entries(value).filter(([key]) => key !== 'cwd')) : undefined;
  if (!validModelCatalogRequest(binding) || typeof value.cwd !== 'string' || !isAbsolute(value.cwd) ||
      value.cwd.length > 4096 || value.cwd.includes('\0')) throw new CatalogError('INVALID_REQUEST');
  return value;
}

// Rebuild the child environment even for standalone invocations. No API keys,
// endpoint overrides, NODE_OPTIONS, runtime profiles or shell functions travel.
export function catalogEnvironment(env) {
  return Object.fromEntries(['PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TERM', 'SHELL', 'USER', 'LOGNAME',
    'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME', 'CODEX_HOME', 'CLAUDE_CONFIG_DIR']
    .filter(key => typeof env[key] === 'string').map(key => [key, env[key]]));
}

function command(env, key, fallback) {
  const value = env[key] || fallback;
  if (typeof value !== 'string' || value.includes('\0') || value.startsWith('-') || /\s/.test(value) && !isAbsolute(value)) throw new CatalogError('CATALOG_UNAVAILABLE');
  return value;
}

function row(value) {
  if (!record(value) || !id(value.id) || !readable(value.label) || value.description !== undefined && !readable(value.description, 1024) ||
      value.efforts !== undefined && (!Array.isArray(value.efforts) || value.efforts.length > 10 || new Set(value.efforts).size !== value.efforts.length || value.efforts.some(effort => !EFFORTS.includes(effort))) ||
      value.defaultEffort !== undefined && !value.efforts?.includes(value.defaultEffort)) throw new CatalogError('CATALOG_PROTOCOL');
  return value;
}

function rows(values) {
  const seen = new Set();
  return values.map(value => {
    const model = row(value);
    if (seen.has(model.id)) throw new CatalogError('CATALOG_PROTOCOL');
    seen.add(model.id);
    return model;
  });
}

async function antigravity(request, runtime) {
  const process = runtime.process(command(runtime.env, 'FLYWHEEL_AGY_BIN', 'agy'), ['models'], runtime.options);
  try {
    const output = await process.text();
    const entries = output
      .split(/\r?\n/)
      .map(line => {
        const trimmed = line.trim();
        const m = /^([a-z][a-z0-9._-]+)(?:\s+(.+))?$/.exec(trimmed);
        if (!m) return null;
        const rawLabel = m[2]?.trim() ?? '';
        return { id: m[1], label: rawLabel && readable(rawLabel) ? rawLabel : m[1] };
      })
      .filter(Boolean);
    if (!entries.length) throw new CatalogError('CATALOG_PROTOCOL');
    const models = entries.slice(0, MODEL_CATALOG_LIMIT).map(({ id: value, label }) => {
      // These are observed CLI IDs, never a static fallback. Only the three
      // pinned Orchestrator IDs and valid adjustable slugs prove an effort.
      const match = /^gemini-4-(hydrogen|neon|carbon|argon)-(low|medium|high|xhigh)$/.exec(value);
      const effort = ['gemini-pro-orchestrator-neon', 'gemini-pro-orchestrator', 'gemini-pro-orchestrator-argon'].includes(value) ? ''
        : match && !(match[2] === 'xhigh' && ['carbon', 'argon'].includes(match[1])) ? match[2] : undefined;
      return { id: value, label, ...(effort === undefined ? {} : { efforts: [effort], defaultEffort: effort }) };
    });
    return { models: rows(models), truncated: entries.length > MODEL_CATALOG_LIMIT };
  } finally { await process.close(); }
}

async function codex(request, runtime) {
  const process = runtime.process(command(runtime.env, 'FLYWHEEL_CODEX_BIN', 'codex'), ['app-server', '--stdio'], runtime.options);
  try {
    const initialized = await process.request('initialize', { clientInfo: { name: 'flywheel-model-catalog', version: '1' }, capabilities: { experimentalApi: false } });
    if (!record(initialized)) throw new CatalogError('CATALOG_PROTOCOL');
    process.notify('initialized');
    const models = [], cursors = new Set();
    let cursor;
    for (let page = 0; page < 8; page++) {
      const result = await process.request('model/list', { limit: 100, includeHidden: false,
        ...(request.provider ? { modelProvider: request.provider } : {}), ...(cursor ? { cursor } : {}) });
      if (!record(result) || !Array.isArray(result.data) || result.data.length > 256 ||
          result.nextCursor !== undefined && result.nextCursor !== null && !readable(result.nextCursor, 4096)) throw new CatalogError('CATALOG_PROTOCOL');
      for (const value of result.data) {
        if (!record(value) || !id(value.model) || !Array.isArray(value.supportedReasoningEfforts)) throw new CatalogError('CATALOG_PROTOCOL');
        models.push({ id: value.model, label: value.displayName,
          ...(value.description ? { description: value.description } : {}),
          efforts: value.supportedReasoningEfforts.map(item => item?.reasoningEffort),
          ...(value.defaultReasoningEffort === undefined || value.defaultReasoningEffort === null ? {} : { defaultEffort: value.defaultReasoningEffort }) });
      }
      if (models.length > MODEL_CATALOG_LIMIT || models.length === MODEL_CATALOG_LIMIT && result.nextCursor) {
        return { models: rows(models.slice(0, MODEL_CATALOG_LIMIT)), truncated: true };
      }
      if (!result.nextCursor) return { models: rows(models), truncated: false };
      if (cursors.has(result.nextCursor)) throw new CatalogError('CATALOG_PROTOCOL');
      cursors.add(result.nextCursor);
      cursor = result.nextCursor;
    }
    return { models: rows(models), truncated: true };
  } finally { await process.close(); }
}

async function loadClaudeSDK() {
  const require = createRequire(import.meta.url);
  const manifestPath = require.resolve('dsh-subagent-claude-code-pro/package.json');
  const manifest = require(manifestPath);
  if (manifest.version !== '0.7.1' || manifest.dependencies?.['@anthropic-ai/claude-agent-sdk'] !== '0.3.326') throw new CatalogError('CATALOG_UNAVAILABLE');
  return import(pathToFileURL(createRequire(manifestPath).resolve('@anthropic-ai/claude-agent-sdk')).href);
}

async function claude(request, runtime) {
  const sdk = await runtime.loadClaude();
  if (typeof sdk.query !== 'function') throw new CatalogError('CATALOG_UNAVAILABLE');
  let release, owned;
  const held = new Promise(resolve => { release = resolve; });
  const controller = new AbortController();
  const abort = () => controller.abort();
  runtime.options.signal?.addEventListener('abort', abort, { once: true });
  const timer = setTimeout(abort, runtime.options.timeoutMs ?? 10000);
  const prompt = (async function* () { await held; })(); // Never yields any SDKUserMessage.
  let query;
  try {
    if (runtime.options.signal?.aborted) throw new CatalogError('CANCELLED');
    query = sdk.query({ prompt, options: {
      cwd: request.cwd, env: runtime.options.env, pathToClaudeCodeExecutable: command(runtime.env, 'FLYWHEEL_CLAUDE_BIN', 'claude'),
      abortController: controller, persistSession: false, settingSources: [], tools: [], mcpServers: {}, plugins: [],
      permissionMode: 'dontAsk', settings: { disableAllHooks: true }, extraArgs: { 'strict-mcp-config': null },
      // Own cleanup and bound both streams, including the SDK initialize frame.
      spawnClaudeCodeProcess(options) {
        owned = runtime.process(options.command, options.args, { ...runtime.options, env: options.env });
        // SDK consumes stdout itself. The observer only counts bytes.
        owned.child.stdout.removeAllListeners('data');
        owned.child.stdout.on('data', chunk => owned.count(chunk));
        return owned.child;
      },
    } });
    const values = await query.supportedModels();
    if (controller.signal.aborted || owned?.failure) throw new CatalogError(owned?.failure ?? 'CATALOG_TIMEOUT');
    if (!Array.isArray(values)) throw new CatalogError('CATALOG_PROTOCOL');
    const models = values.slice(0, MODEL_CATALOG_LIMIT).map(value => ({ id: value?.value, label: value?.displayName,
      ...(value?.description ? { description: value.description } : {}),
      ...(value?.supportedEffortLevels === undefined ? {} : { efforts: value.supportedEffortLevels }) }));
    return { models: rows(models), truncated: values.length > MODEL_CATALOG_LIMIT,
      limitations: ['Claude metadata is initialized with user/project settings, hooks, plugins and MCP disabled; execution settings may further restrict the catalog.'] };
  } finally {
    clearTimeout(timer);
    runtime.options.signal?.removeEventListener('abort', abort);
    query?.close();
    controller.abort();
    release();
    await owned?.close();
  }
}

export async function modelCatalog(input, { env = process.env, signal, timeoutMs = 10000,
  process: processFactory = (command, args, options) => new CatalogProcess(command, args, options), loadClaude = loadClaudeSDK } = {}) {
  const request = validateCatalogRequest(input);
  const { cwd, ...binding } = request;
  const result = { ...binding, status: 'unsupported', source: SOURCES[request.harness] ?? 'unsupported', scope: 'native-client-catalog',
    models: [], truncated: false, inferencePerformed: false, accountEntitlement: 'unknown',
    limitations: ['Catalog membership does not verify account access, remaining quota or execution eligibility.'] };
  const probes = { antigravity, 'codex-pro': codex, 'claude-code-pro': claude };
  const probe = Object.hasOwn(probes, request.harness) ? probes[request.harness] : undefined;
  if (!probe) {
    result.limitations.push('This binding has no verified read-only catalog transport; enter an exact model ID manually.');
    return result;
  }
  try {
    if (signal?.aborted) throw new CatalogError('CANCELLED');
    const observed = await probe(request, { env, process: processFactory, loadClaude, options: { cwd, env: catalogEnvironment(env), signal, timeoutMs } });
    result.models = observed.models;
    result.truncated = observed.truncated;
    result.status = 'available';
    result.limitations.push(...(observed.limitations ?? []));
    if (result.truncated) result.limitations.push('The catalog reached its bounded row/page limit; additional models may exist.');
  } catch (error) {
    const code = error instanceof CatalogError ? error.code : 'CATALOG_UNAVAILABLE';
    result.status = 'unavailable';
    result.truncated = code === 'OUTPUT_LIMIT';
    result.error = { code, message: 'The native model catalog could not be read safely; exact model IDs can still be entered manually.' };
  }
  return result;
}
