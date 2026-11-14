// Runs inside the owned SDK process. All observations use public runtime seams.
import { readFile, mkdir, open, lstat, realpath } from 'node:fs/promises';
import { constants } from 'node:fs';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';
import { createSdkPtcComposition, validateSdkPtcAdmission, inspectPtcBundle } from 'dsh-preset-generator/composition';
import { nativeDigest, validateNative } from './native.js';
import { verifyGuiLease } from './native-resources.js';

export function nativeOverlay(request, runtimeProfile = 'sdk') {
  validateNative(request.native);
  const options = request.native;
  const composition = createSdkPtcComposition({ runtimeProfile, preset: options.preset });
  const enabled = options.agents?.enabled === true;
  const maxConcurrent = options.agents?.maxConcurrent ?? 8;
  const maxDepth = options.agents?.maxDepth ?? 2;
  const overlay = [
    ...composition.overlay,
    { id: 'subagent', config: { maxActiveSubagents: maxConcurrent, maxDepth } },
    ...['subagent-spawn-in-process', 'subagent-fork-in-process'].map(id => ({ id, disabled: !enabled })),
    { id: 'tool-subagent-control', disabled: !enabled, config: { interrupt: true } },
    ...[['tool-subagent', 'spawn', 'subagent'], ['tool-subagent-fork', 'fork', 'subagent_fork']].map(([id, provider, toolName]) =>
      ({ id, disabled: !enabled, config: { provider, toolName, maxDepth, modelSelection: false } })),
    { id: 'agent-team', config: { defaultEnabled: options.team?.enabled === true, maxTasks: 256 } },
    { id: 'tool-agent-team', disabled: options.team?.enabled !== true },
    { id: 'workflow-ptc', disabled: options.workflows?.enabled !== true,
      config: { provider: 'spawn', maxConcurrentAgents: maxConcurrent, maxTotalAgents: 1000 } },
    { id: 'tool-workflow', disabled: options.workflows?.enabled !== true },
    { id: 'tool-web', disabled: !options.web?.search && !options.web?.fetch,
      config: { search: options.web?.search === true, fetch: options.web?.fetch === true } },
  ];
  return { composition, overlay };
}

const PROBE_IMAGE = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGNgAAIAAAUAAXpeqz8AAAAASUVORK5CYII=', 'base64');

/** Execute the actual selected PTC path, without a model request or desktop action. */
export async function probePtc(ctx, agent, composition, signal = AbortSignal.timeout(10000)) {
  if (!ctx.get('tools') || !ctx.get('systemPrompt') || !ctx.get('ptcRuntime')) throw new Error('Native DSH requires public PTC runtime services');
  const assembly = await ctx.systemPrompt.assemble({ scope: agent });
  const schemas = assembly.tools.map(tool => tool.name);
  if (schemas.length !== 1 || schemas[0] !== 'run_code') throw new Error('Native DSH requires exactly the PTC run_code executor');
  if (ctx.ptcRuntime.language !== 'typescript') throw new Error('Native DSH requires the TypeScript PTC runtime');
  const store = ctx.get('attachments');
  const image = store ? (await store.saveImages([{ data: PROBE_IMAGE, mediaType: 'image/png', name: 'flywheel-ptc-probe.png' }]))[0] : undefined;
  const marker = `flywheel_probe_${nativeDigest(composition).slice(0, 16)}`;
  const disposers = [];
  let result;
  try {
    const definition = (name, images) => ({
      name, description: 'Internal no-effect initialization probe.', parameters: {},
      output: { schema: { type: 'object', required: ['marker', '__handle'], properties: { marker: { type: 'string' },
        __handle: { type: 'string' } }, additionalProperties: false },
        render: () => [{ type: 'text', text: marker }, ...Array.from({ length: image ? images : 0 }, () => ({ type: 'image', attachment: image }))] },
      execute: async () => ({ marker, __handle: 'probe-handle' }),
    });
    disposers.push(agent.ctx.tools.register(definition('flywheel_ptc_probe', 1)));
    disposers.push(agent.ctx.tools.register(definition('flywheel_ptc_multi_probe', 2)));
    // The new executor reserves read_image for conversion into its PtcImage.
    // This temporary agent-scoped tool returns only our known in-memory image;
    // it tests transport identity, not filesystem access or screenshot capture.
    if (image) disposers.push(agent.ctx.tools.register(definition('read_image', 1)));
    const code = `await (async () => {
const v = await tools.flywheel_ptc_probe({});
const many = await tools.flywheel_ptc_multi_probe({});
if (v.__handle !== 'probe-handle' || v.marker !== ${JSON.stringify(marker)} || many.__handle !== 'probe-handle') throw new Error('canonical PTC value lost');
if (typeof inspect === 'function') {
  if (typeof Image !== 'function') throw new Error('inspect runtime has no Image identity');
  inspect(${JSON.stringify(marker)});
  inspect('flywheel-inspect-v1');
  ${image ? "const image = await tools.read_image({}); if (!(image instanceof Image)) throw new Error('PTC Image identity lost'); inspect(image); inspect('flywheel-image-identity-v1');" : ''}
} else {
  console.log(${JSON.stringify(marker)});
  console.log('flywheel-deferred-v1');
}
})();`;
    result = await ctx.tools.execute({ agent, signal, calls: [{ type: 'tool-call', id: marker, name: 'run_code', arguments: JSON.stringify({ code, description: 'Verify PTC canonical values, multi-image projection and selected output transport without model inference' }) }] });
  } finally { for (const dispose of disposers.reverse()) dispose(); }
  const content = result.results?.[0]?.result?.content ?? [];
  const text = content.filter(block => block.type === 'text').map(block => block.text).join('\n');
  if (result.results?.[0]?.result?.isError !== false || !text.includes(marker)) throw new Error('The selected PTC executor failed its canonical-value/output probe');
  const modern = text.includes('flywheel-inspect-v1');
  const legacy = text.includes('flywheel-deferred-v1');
  if (modern === legacy) throw new Error('PTC output transport was not attributable to one runtime contract');
  const sameImage = block => block.type === 'image' && block.attachment?.attachmentId === image?.attachmentId;
  const deferredCount = result.additionalContexts?.flatMap(message => message.content ?? []).filter(sameImage).length ?? 0;
  const inspected = Boolean(image && content.some(sameImage) && text.includes('flywheel-image-identity-v1'));
  const imagesPassed = Boolean(image && deferredCount === 3 && (!modern || inspected));
  return { compositionDigest: composition.digest, runtimeProfile: composition.runtimeProfile, executor: 'ptc', activeExecutors: ['ptc'],
    tools: schemas, outputContract: modern ? 'inspect-image-v1' : 'deferred-context-v1', probeStatus: 'passed',
    imageProbeStatus: image ? (imagesPassed ? 'passed' : 'failed') : 'not-run',
    probes: { canonicalValue: 'passed', multiImageProjection: image ? (deferredCount === 3 ? 'passed' : 'failed') : 'not-run',
      imageIdentity: image && modern ? (inspected ? 'passed' : 'failed') : 'not-run',
      inspectedImageProjection: image && modern ? (inspected ? 'passed' : 'failed') : 'not-run' } };
}

async function runtimeBundleEvidence() {
  // Resolve from the running SDK entry, not the Flywheel dependency tree.
  // A host with an opaque loader still has behavioral admission; its receipt
  // explicitly omits static bundle evidence rather than inventing a version.
  try {
    const require = createRequire(process.argv[1]);
    const manifestPath = require.resolve('@deepseek-ai/dsh-tool-executor-ptc/package.json');
    const manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
    const entry = require.resolve('@deepseek-ai/dsh-tool-executor-ptc');
    return inspectPtcBundle({ packageJson: manifest, files: { 'entry.js': await readFile(entry, 'utf8') } });
  } catch { return null; }
}

/** Private bounded journal: queue reservations are bounded before I/O begins. */
export async function createNativeJournal(path, identity) {
  const parent = dirname(path);
  const stat = await lstat(parent);
  if (!stat.isDirectory() || await realpath(parent) !== parent || (stat.mode & 0o022) || (process.getuid && stat.uid !== process.getuid())) {
    throw new Error('Native journal requires an owned physical directory');
  }
  const file = await open(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
  let reserved = 0, reservedBytes = 0, written = 0, writtenBytes = 0, error, closed = false, tail = Promise.resolve();
  const fail = cause => { error ??= cause instanceof Error ? cause : new Error(String(cause)); return error; };
  const snapshot = () => ({ complete: !error && reserved === written, events: written, bytes: writtenBytes,
    ...(error ? { error: error.message.slice(0, 2048) } : {}) });
  return {
    emit(event) {
      if (error) return Promise.reject(error);
      if (closed) return Promise.reject(fail(new Error('Native journal is already closed')));
      let line;
      try { line = JSON.stringify({ ...event, ...identity, version: 1, journalSequence: reserved + 1 }) + '\n'; }
      catch (cause) { return Promise.reject(fail(cause)); }
      const bytes = Buffer.byteLength(line);
      if (reserved >= 4096 || reservedBytes + bytes > 2 * 1024 * 1024) {
        return Promise.reject(fail(new Error('Native journal exceeded its 4096 event / 2 MiB budget')));
      }
      reserved += 1; reservedBytes += bytes;
      const pending = tail.then(async () => {
        if (error) throw error;
        await file.writeFile(line); await file.sync();
        written += 1; writtenBytes += bytes;
      });
      tail = pending.catch(cause => { fail(cause); });
      return pending;
    },
    async flush() { await tail; if (error) throw error; },
    snapshot,
    async close() {
      if (!closed) { closed = true; await tail; await file.close().catch(fail); }
      return snapshot();
    },
  };
}

async function writePrivateJson(path, value) {
  const file = await open(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
  try { await file.writeFile(JSON.stringify(value)); await file.sync(); } finally { await file.close(); }
  const directory = await open(dirname(path), constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
  try { await directory.sync(); } finally { await directory.close(); }
}

const AGENT_TOOLS = ['subagent', 'subagent_fork', 'list_agents', 'send_message', 'wait_agent', 'interrupt_agent'];
const TEAM_TOOLS = ['team_task_create', 'team_task_list', 'team_task_get', 'team_task_update'];

export async function createNativePolicy(ctx, config) {
  if (!config.native) return null;
  validateNative(config.native);
  const admitted = new Map(), ready = new Set(), lineage = new Map(), activations = new Map();
  const eventsPath = join(dirname(config.receiptPath), 'native-events.jsonl');
  const cleanupPath = join(dirname(config.receiptPath), 'native-cleanup.json');
  const journal = await createNativeJournal(eventsPath, { requestId: config.requestId, nonce: config.nonce,
    rootSessionId: config.sessionId, invocationId: config.managed?.invocationId ?? config.requestId, runId: config.managed?.runId ?? null });
  let lcu, observed, rootAgent, observationFailure;
  const emit = event => journal.emit(event);
  const isLive = agent => agent && String(agent.id) === String(agent.session.id) && ctx.get('agents')?.get(String(agent.id)) === agent;
  const liveLineage = agent => {
    if (!isLive(agent) || admitted.get(String(agent.id)) !== agent || !isLive(rootAgent)) return false;
    const seen = new Set();
    for (let member = agent; member; member = admitted.get(String(member.session.header.parentSession ?? ''))) {
      const record = lineage.get(String(member.id));
      if (seen.has(member) || admitted.get(String(member.id)) !== member || !record ||
          record.parentId !== String(member.session.header.parentSession ?? '') || record.origin !== member.session.header.origin) return false;
      if (member === rootAgent) return String(member.id) === config.sessionId;
      if (member.session.header.origin !== 'subagent') return false;
      seen.add(member);
    }
    return false;
  };
  const identityOf = agent => {
    if (!liveLineage(agent)) throw new Error('LCU caller is outside the live admitted native tree');
    if (agent !== rootAgent && ready.has(String(agent.id)) && ctx.get('subagents')?.rootOf(agent) !== rootAgent) {
      throw new Error('LCU caller has no runtime ownership in the admitted native tree');
    }
    return { runId: config.managed.runId, invocationId: config.managed.invocationId, rootSessionId: config.sessionId,
      ...(config.managed.candidateId ? { candidateId: config.managed.candidateId } : {}) };
  };
  // Register cleanup before a host dependency can fail initialization.
  ctx.effect(() => async () => {
    let receipts = [], cleanupError;
    try { receipts = lcu ? await lcu.close('owned SDK shutdown') : []; }
    catch (cause) { cleanupError = cause instanceof Error ? cause.message : String(cause); }
    const facts = await journal.close();
    if (observationFailure) {
      facts.complete = false;
      facts.error ??= (observationFailure instanceof Error ? observationFailure.message : String(observationFailure)).slice(0, 2048);
    }
    await writePrivateJson(cleanupPath, { version: 1, requestId: config.requestId, nonce: config.nonce,
      cleanup: receipts, lcu: lcu?.snapshot() ?? null, journal: facts,
      ...(cleanupError ? { error: cleanupError.slice(0, 2048) } : {}) });
  });
  if (config.native.lcu?.enabled) {
    if (!config.managed?.lease) throw new Error('Managed LCU requires an owned desktop lease');
    const { createManagedLcu } = await import('./managed-lcu.js');
    lcu = await createManagedLcu(ctx, { options: config.native.lcu.config ?? {}, managed: config.managed,
      rootSessionId: config.sessionId, identityOf, verifyLease: () => verifyGuiLease(config.managed.lease), onEvent: emit });
  }
  const checkTools = async (agent, signal = AbortSignal.timeout(10000)) => {
    const registered = ctx.tools.schemas(agent).map(tool => tool.name).sort();
    const toolSet = await ctx.tools.resolve({ agent, modelSelection: agent.model.current(), signal });
    let capabilities;
    try { capabilities = ctx.tools.schemas(agent, toolSet).map(tool => tool.name).sort(); }
    finally { toolSet.dispose(); }
    const required = [
      ...(config.native.agents?.enabled ? AGENT_TOOLS : []), ...(config.native.team?.enabled ? TEAM_TOOLS : []),
      ...(config.native.workflows?.enabled ? ['workflow'] : []),
      ...(config.native.web?.search ? ['web_search'] : []), ...(config.native.web?.fetch ? ['web_fetch'] : []),
      ...(lcu ? ['lcu_cua'] : []),
    ];
    const forbidden = [
      ...(!config.native.agents?.enabled ? AGENT_TOOLS : []), ...(!config.native.team?.enabled ? TEAM_TOOLS : []),
      ...(!config.native.workflows?.enabled ? ['workflow'] : []),
      ...(!config.native.web?.search ? ['web_search'] : []), ...(!config.native.web?.fetch ? ['web_fetch'] : []),
      ...(lcu ? ['cua', 'js', 'js_reset'] : ['lcu_cua', 'cua']),
    ];
    const missing = required.filter(name => !capabilities.includes(name));
    const remaining = forbidden.filter(name => registered.includes(name));
    if (missing.length) throw new Error(`Native capability tools did not register: ${missing.join(', ')}`);
    if (remaining.length) throw new Error(`Unadmitted native capability tools remain registered: ${remaining.join(', ')}`);
    return capabilities;
  };
  const checkRoute = agent => {
    const selection = agent.model.current();
    if (!selection || selection.provider !== config.provider || selection.model !== config.model ||
        (config.reasoningEffort !== undefined && selection.reasoningEffort !== config.reasoningEffort)) {
      throw new Error('Native Agent route or reasoning effort differs from its admitted account/model');
    }
  };
  const checkDepth = agent => {
    if (agent === rootAgent) return;
    const depth = member => Math.max(member.session.header.delegationDepth ?? 0, member.options.subagentDepth ?? 0);
    const parent = admitted.get(String(agent.session.header.parentSession ?? ''));
    const current = depth(agent), parentDepth = parent && depth(parent), rootDepth = rootAgent && depth(rootAgent);
    if (!Number.isSafeInteger(current) || !Number.isSafeInteger(parentDepth) || !Number.isSafeInteger(rootDepth) ||
        current < parentDepth + 1 || current - rootDepth > (config.native.agents?.maxDepth ?? 2)) {
      throw new Error('Native child exceeds its admitted tree delegation depth');
    }
  };
  // These public events include external providers that the SDK JSON-RPC local
  // Session notifications do not expose. Bind each epoch to a real live parent
  // at start, then retain that immutable attribution for its terminal event.
  for (const [name, type] of [['subagent/start', 'subagent.started'], ['subagent/end', 'subagent.finished']]) {
    ctx.on(name, async info => {
      const parent = admitted.get(String(info?.parentSessionId ?? ''));
      if (!parent) return;
      try {
        if (!info || typeof info.runId !== 'string' || !info.runId || typeof info.id !== 'string' || !info.id ||
            typeof info.provider !== 'string' || !info.provider || typeof info.local !== 'boolean') throw new Error('Malformed public native lifecycle identity');
        const child = { runId: info.runId, provider: info.provider, parentSessionId: info.parentSessionId, id: info.id, local: info.local };
        const existing = activations.get(info.runId);
        if (type === 'subagent.started') {
          if (!config.native.agents?.enabled || !ready.has(String(parent.id)) || !liveLineage(parent)) throw new Error('Native lifecycle start has no admitted live parent');
          if (info.local && (admitted.get(info.id) !== ctx.get('agents')?.get(info.id) || !ready.has(info.id))) throw new Error('Local native lifecycle does not name an admitted Agent');
          if (existing && nativeDigest(existing) !== nativeDigest(child)) throw new Error('Native lifecycle epoch changed identity');
          activations.set(info.runId, child);
        } else {
          if (!existing || nativeDigest(existing) !== nativeDigest(child) || typeof info.stopReason !== 'string' || !info.stopReason) throw new Error('Native lifecycle terminal has no matching admitted epoch');
          child.stopReason = info.stopReason;
        }
        await emit({ type, child });
      } catch (cause) {
        // @emit observers are contained by DSH. Persist a failure and make the
        // next awaited step/turn-stop fail instead of trusting a swallowed throw.
        await emit({ type: 'native.observation-failed', matched: false,
          error: (cause instanceof Error ? cause.message : String(cause)).slice(0, 2048) });
        observationFailure ??= cause;
        throw cause;
      }
    });
  }
  return {
    isChild(agent) {
      const parent = admitted.get(String(agent.session.header.parentSession ?? ''));
      return config.native.agents?.enabled === true && isLive(agent) && agent.session.header.origin === 'subagent' &&
        parent !== undefined && ready.has(String(parent.id)) && isLive(rootAgent) && lineage.has(String(parent.id));
    },
    async admit(agent, root) {
      const id = String(agent.session.id);
      const previous = admitted.get(id), previousLineage = lineage.get(id), previouslyReady = ready.has(id);
      try {
        if (!isLive(agent) || (root ? id !== config.sessionId || rootAgent && rootAgent !== agent : !this.isChild(agent))) throw new Error('Native DSH refuses an unrelated Agent');
        const record = { parentId: String(agent.session.header.parentSession ?? ''), origin: agent.session.header.origin,
          depth: agent.session.header.delegationDepth ?? 0 };
        if (previous && previous !== agent && (root || isLive(previous) || nativeDigest(previousLineage) !== nativeDigest(record))) throw new Error('Native Agent identity or lineage was replaced');
        checkRoute(agent);
        ready.delete(id);
        admitted.set(id, agent);
        lineage.set(id, record);
        if (root) rootAgent = agent;
        checkDepth(agent);
        if (lcu) {
          const info = await ctx.get('llm')?.resolveModelInfo(config.provider, config.model, AbortSignal.timeout(10000));
          if (!info?.inputModalities?.includes('image')) throw new Error('Managed LCU requires an explicitly image-capable model on the selected account');
          await lcu.attach(agent);
        }
        if (root) {
          const team = ctx.get('agentTeams');
          if (team && team.isEnabled(agent) !== Boolean(config.native.team?.enabled)) await team.setEnabled(agent, Boolean(config.native.team?.enabled));
          if (config.native.team?.enabled && !team?.isEnabled?.(agent)) throw new Error('Native DSH Team mode did not become effective');
          const composition = config.composition;
          const probe = await probePtc(ctx, agent, composition);
          const admission = validateSdkPtcAdmission(composition, probe, { requireImages: Boolean(lcu) });
          if (!admission.eligible) throw new Error(`PTC runtime admission failed: ${admission.failures.join(', ')}`);
          const capabilities = await checkTools(agent);
          observed = { ...probe, capabilities, admission, staticEvidence: await runtimeBundleEvidence(), namedPresetMounted: false };
          const binding = { options: config.native, observed, provider: config.provider, model: config.model,
            reasoningEffort: config.reasoningEffort, permissionMode: config.permissionMode, cwd: config.cwd, workspaceCwd: config.workspaceCwd ?? config.cwd };
          observed.runtimeBindingDigest = nativeDigest(binding);
          if (config.managed?.taskDir) {
            const historyDirectory = join(config.managed.taskDir, '.control', 'native-history');
            await mkdir(historyDirectory, { recursive: true, mode: 0o700 });
            for (const directory of [config.managed.taskDir, dirname(historyDirectory), historyDirectory]) {
              const info = await lstat(directory);
              if (!info.isDirectory() || await realpath(directory) !== directory || (info.mode & 0o022) || (process.getuid && info.uid !== process.getuid())) throw new Error('Native history requires owned physical directories');
            }
            const path = join(historyDirectory, nativeDigest(id) + '.json');
            if (config.resuming) {
              const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
              try {
                const info = await file.stat();
                if (!info.isFile() || info.size > 4096 || (info.mode & 0o077) || (process.getuid && info.uid !== process.getuid())) throw new Error('Native history receipt is not a bounded private file');
                const previous = JSON.parse(await file.readFile('utf8'));
                if (previous.version !== 1 || previous.sessionId !== id || previous.digest !== observed.runtimeBindingDigest) throw new Error('Native runtime composition changed; resume withheld before model dispatch');
              } finally { await file.close(); }
            } else await writePrivateJson(path, { version: 1, sessionId: id, digest: observed.runtimeBindingDigest });
          }
        }
        await emit({ type: 'agent.admitted', id, parentId: agent.session.header.parentSession ?? null, matched: true, permissionMode: config.permissionMode });
        ready.add(id);
        agent.ctx.on('agent/pre-step', async (payload, next) => {
          if (observationFailure) throw observationFailure;
          await journal.flush();
          if (!ready.has(id) || !liveLineage(agent)) throw new Error('Native Agent lineage is no longer live');
          if (agent !== rootAgent && ctx.get('subagents')?.rootOf(agent) !== rootAgent) throw new Error('Native Agent has no runtime ownership in the admitted tree');
          checkRoute(agent); checkDepth(agent); await checkTools(agent, payload.signal);
          const assembly = await ctx.systemPrompt.assemble({ scope: agent });
          if (assembly.tools.length !== 1 || assembly.tools[0].name !== 'run_code') throw new Error('Native Agent lost its admitted PTC executor');
          return next();
        });
        agent.ctx.on('agent/turn-stopping', async () => { await journal.flush(); if (observationFailure) throw observationFailure; });
        return root ? { requested: config.native, observed, matched: true } : undefined;
      } catch (cause) {
        ready.delete(id); admitted.delete(id);
        if (previous && previous !== agent) admitted.set(id, previous);
        if (previouslyReady && previous && previous !== agent) ready.add(id);
        if (previousLineage) lineage.set(id, previousLineage); else lineage.delete(id);
        if (rootAgent === agent) rootAgent = undefined;
        await emit({ type: 'agent.admission-rejected', id, parentId: agent.session.header.parentSession ?? null,
          matched: false, beforeModelDispatch: true,
          parentLive: (() => { const parent = admitted.get(String(agent.session.header.parentSession ?? '')); return Boolean(parent && ready.has(String(parent.id)) && liveLineage(parent)); })(),
          error: (cause instanceof Error ? cause.message : String(cause)).slice(0, 2048) }).catch(() => {});
        throw cause;
      }
    },
  };
}
