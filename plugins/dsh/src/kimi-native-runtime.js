import { createHash } from 'node:crypto';
import { spawn, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { readdir, readFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join } from 'node:path';

const execute = promisify(execFile);
export const digest = value => createHash('sha256').update(typeof value === 'string' ? value : JSON.stringify(value)).digest('hex');
export const error = (code, message) => Object.assign(new Error(message), { code });
export const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
export const checkAbort = signal => { if (signal?.aborted) throw typeof signal.reason?.code === 'string' ? signal.reason : error('CANCELLED', 'The owned Kimi invocation was cancelled'); };

async function processTable() {
  const { stdout } = await execute('ps', ['-axo', 'pid=,ppid=,lstart=,stat='], {
    encoding: 'utf8', maxBuffer: 4 * 1024 * 1024, timeout: 2000, env: { ...process.env, LC_ALL: 'C' },
  });
  return stdout.trim().split('\n').map(line => {
    const fields = line.trim().split(/\s+/);
    return { pid: Number(fields[0]), parent: Number(fields[1]), birth: fields.slice(2, 7).join(' '), state: fields[7] };
  }).filter(row => Number.isSafeInteger(row.pid) && row.birth && row.state);
}

/** One foreground local server, sharing only the user's existing credential home. */
export class OwnedKimiRuntime {
  constructor(env, mode) {
    this.env = { ...env, KIMI_CODE_EXPERIMENTAL_TOWER: mode === 'tower' ? '1' : '0' };
    this.home = env.KIMI_CODE_HOME || join(env.HOME || homedir(), '.kimi-code');
    this.processes = new Map();
    this.processInspection = false;
    this.closed = false;
  }
  async sampleProcesses() {
    const table = await processTable();
    let changed = true;
    const root = table.find(row => row.pid === this.child.pid);
    // Never adopt a reused PID after the owned process has exited.
    const owned = new Set();
    if (root && (!this.closed || this.processes.get(root.pid)?.birth === root.birth)) {
      owned.add(root.pid); this.observedRoot = true;
    }
    for (const row of table) if (this.processes.get(row.pid)?.birth === row.birth) owned.add(row.pid);
    while (changed) {
      changed = false;
      for (const row of table) if (!owned.has(row.pid) && owned.has(row.parent)) { owned.add(row.pid); changed = true; }
    }
    for (const row of table) if (owned.has(row.pid)) this.processes.set(row.pid, row);
    this.processInspection = true;
    return table.filter(row => this.processes.get(row.pid)?.birth === row.birth && !row.state.startsWith('Z'));
  }
  async start(signal) {
    checkAbort(signal);
    const started = Date.now();
    this.child = spawn(this.env.FLYWHEEL_KIMI_BIN || 'kimi', ['web', '--no-open', '--host', '127.0.0.1', '--port', '0'], {
      env: this.env, stdio: ['ignore', 'pipe', 'pipe'], detached: false,
    });
    // The CLI banner includes the bearer token. Neither stdout nor stderr is a
    // machine protocol, and no raw startup bytes are forwarded or persisted.
    this.child.stdout.on('data', () => {});
    this.child.stderr.on('data', () => {});
    this.exit = new Promise(resolve => {
      this.child.once('error', cause => { this.spawnError = cause; this.closed = true; resolve(); });
      this.child.once('exit', (code, signalName) => { this.closed = true; this.exitCode = code; this.exitSignal = signalName; resolve(); });
    });
    this.sampler = setInterval(() => { this.sampleProcesses().catch(() => { this.processInspection = false; }); }, 500);
    this.sampler.unref();
    await this.sampleProcesses();
    const directory = join(this.home, 'server', 'instances');
    while (Date.now() - started < 20_000) {
      checkAbort(signal);
      if (this.closed) throw error('RUNTIME_START', this.spawnError ? 'Cannot start the configured Kimi CLI' : 'Kimi web exited before registering its owned server');
      let names = [];
      try { names = await readdir(directory); } catch (cause) { if (cause.code !== 'ENOENT') throw error('RUNTIME_REGISTRY', 'Cannot read the Kimi server instance registry'); }
      for (const name of names.filter(name => name.endsWith('.json'))) {
        let entry;
        try { entry = JSON.parse(await readFile(join(directory, name), 'utf8')); } catch { continue; }
        if (entry.pid !== this.child.pid || entry.host !== '127.0.0.1' || !Number.isSafeInteger(entry.port) || entry.port < 1 || entry.port > 65535
            || typeof entry.server_id !== 'string' || entry.started_at < started - 2000) continue;
        const token = (await readFile(join(this.home, 'server.token'), 'utf8')).trim();
        if (!token || /[\s,]/.test(token) || token.length > 4096) throw error('RUNTIME_AUTH', 'The existing Kimi server token is missing or invalid');
        this.token = token;
        this.base = `http://127.0.0.1:${entry.port}`;
        const meta = await this.request('/api/v1/meta', undefined, { signal });
        if (meta.server_id !== entry.server_id || meta.dangerous_bypass_auth !== false || meta.backend !== 'v2' || meta.capabilities?.websocket !== true) {
          throw error('RUNTIME_IDENTITY', 'The authenticated server does not match the newly owned local runtime');
        }
        this.meta = meta;
        await this.sampleProcesses();
        return;
      }
      await pause(40);
    }
    throw error('RUNTIME_START', 'Kimi web did not register an authenticated local server in time');
  }
  async request(path, body, { signal, raw = false, timeout = 5000 } = {}) {
    const bounded = AbortSignal.timeout(timeout);
    const response = await fetch(this.base + path, {
      method: body === undefined ? 'GET' : 'POST', headers: { Authorization: `Bearer ${this.token}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: signal ? AbortSignal.any([signal, bounded]) : bounded,
    });
    const reader = response.body.getReader();
    const chunks = []; let bytes = 0;
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        bytes += value.byteLength;
        if (bytes > 8 * 1024 * 1024) { await reader.cancel(); throw error('RESPONSE_LIMIT', 'Kimi response exceeds the bounded adapter limit'); }
        chunks.push(Buffer.from(value));
      }
    } finally { reader.releaseLock(); }
    const text = Buffer.concat(chunks).toString('utf8');
    let json;
    try { json = JSON.parse(text); } catch { throw error('RUNTIME_PROTOCOL', 'Kimi returned a non-JSON response'); }
    if (!response.ok || (!raw && json.code !== 0)) {
      // Server errors can include prompts, account details and attachment paths.
      // Stable machine code is sufficient for the receipt; do not echo bodies.
      throw error('KIMI_API_' + (json.code ?? response.status), `Kimi rejected ${path.split('?')[0]} (code ${json.code ?? response.status})`);
    }
    return raw ? json : json.data;
  }
  async stop() {
    if (this.sampler) clearInterval(this.sampler);
    if (!this.child) return { exited: true, processesConfirmed: true, forced: false };
    let inspected = this.processInspection, forced = false;
    try { await this.sampleProcesses(); inspected = true; } catch { inspected = false; }
    if (!this.closed && this.base) {
      try { await this.request('/api/v1/shutdown', {}, { timeout: 2000 }); } catch { /* Exit below remains authoritative. */ }
    }
    let deadline = Date.now() + 3500;
    while (!this.closed && Date.now() < deadline) await pause(30);
    if (!this.closed) { forced = true; this.child.kill('SIGTERM'); }
    deadline = Date.now() + 1500;
    while (!this.closed && Date.now() < deadline) await pause(30);
    if (!this.closed) { forced = true; this.child.kill('SIGKILL'); await Promise.race([this.exit, pause(1000)]); }
    let live = [];
    try { live = await this.sampleProcesses(); } catch { inspected = false; }
    for (const row of live) {
      forced = true;
      try { process.kill(row.pid, 'SIGTERM'); } catch { /* Recheck by birth before escalation. */ }
    }
    if (live.length) await pause(200);
    try { live = await this.sampleProcesses(); } catch { inspected = false; }
    for (const row of live) { try { process.kill(row.pid, 'SIGKILL'); } catch { /* Final table determines certainty. */ } }
    if (live.length) await pause(100);
    try { live = await this.sampleProcesses(); } catch { inspected = false; }
    return { exited: this.closed, processesConfirmed: inspected && (!this.child.pid || Boolean(this.spawnError) || this.observedRoot === true) && live.length === 0, forced,
      observedProcesses: this.processes.size, exitCode: this.exitCode ?? null, signal: this.exitSignal ?? null };
  }
}

function dereference(document, schema, seen = new Set()) {
  if (!schema || typeof schema !== 'object') return {};
  if (schema.$ref?.startsWith('#/') && !seen.has(schema.$ref)) {
    seen.add(schema.$ref);
    return dereference(document, schema.$ref.slice(2).split('/').reduce((node, key) => node?.[key], document), seen);
  }
  const properties = { ...(schema.properties || {}) };
  for (const item of [...(schema.allOf || []), ...(schema.anyOf || []), ...(schema.oneOf || [])]) Object.assign(properties, dereference(document, item, new Set(seen)).properties);
  return { ...schema, properties };
}

/** Match actual runtime contracts; no semantic-version inference is used. */
export function inspectKimiSchemas(openapi, asyncapi, mode) {
  const paths = ['/api/v1/meta', '/api/v1/sessions', '/api/v1/sessions/{session_id}', '/api/v1/sessions/{session_id}/profile',
    '/api/v1/sessions/{session_id}/environment', '/api/v1/sessions/{session_id}/prompts',
    '/api/v1/sessions/{session_id}/agents/{agent_id}/turns', '/api/v1/shutdown', '/api/v1/remote-control'];
  if (!String(openapi.openapi).startsWith('3.') || paths.some(path => !openapi.paths?.[path])) throw error('UNSUPPORTED_SCHEMA', 'Kimi OpenAPI is missing a required local lifecycle route');
  const properties = (path, method = 'post') => dereference(openapi, openapi.paths[path]?.[method]?.requestBody?.content?.['application/json']?.schema).properties || {};
  const creation = properties('/api/v1/sessions');
  const config = dereference(openapi, creation.agent_config).properties || {};
  const prompt = properties('/api/v1/sessions/{session_id}/prompts');
  const profile = dereference(openapi, properties('/api/v1/sessions/{session_id}/profile').agent_config).properties || {};
  if (['model', 'thinking', 'permission_mode', 'swarm_mode', 'profile'].some(key => !config[key])
      || ['content', 'prompt_id', 'disabled_tools'].some(key => !prompt[key])
      || (mode === 'tower' && ['tower_mode', 'tower_base'].some(key => !profile[key]))) {
    throw error('UNSUPPORTED_SCHEMA', 'Kimi request schemas do not describe the selected native profile');
  }
  const messages = new Set(Object.values(asyncapi.components?.messages || {}).map(item => item.name));
  const required = ['subscribe', 'response', 'agent', 'task', 'task.list', 'task.stop', 'turns', 'turn.started', 'turn.ended', 'interaction', 'ping', 'pong'];
  if (asyncapi.channels?.kimiCodeWebSocket?.address !== '/api/v1/ws' || required.some(name => !messages.has(name))) {
    throw error('UNSUPPORTED_SCHEMA', 'Kimi AsyncAPI does not describe the required task and conversation lifecycle');
  }
  return { protocol: 'kimi-server/v1', openapiDigest: digest(openapi), asyncapiDigest: digest(asyncapi), inspectedRoutes: paths, inspectedMessages: required };
}

export class KimiSocket {
  constructor(runtime, receive) { this.runtime = runtime; this.receive = receive; this.pending = new Map(); this.sequence = 0; this.lastMessage = Date.now(); }
  async open(signal) {
    this.socket = new WebSocket(this.runtime.base.replace('http:', 'ws:') + '/api/v1/ws', ['kimi-code.bearer.' + this.runtime.token]);
    this.socket.addEventListener('message', event => {
      this.lastMessage = Date.now();
      try {
        if (typeof event.data !== 'string' || Buffer.byteLength(event.data) > 2 * 1024 * 1024) throw error('WS_PROTOCOL', 'Kimi sent an invalid or oversized WebSocket frame');
        const frame = JSON.parse(event.data);
        if (frame.type === 'ping') { this.socket.send(JSON.stringify({ type: 'pong' })); return; }
        if (frame.type === 'response') {
          const waiter = this.pending.get(frame.request_id);
          if (waiter) { clearTimeout(waiter.timer); this.pending.delete(frame.request_id); frame.code === 0 ? waiter.resolve(frame) : waiter.reject(error('KIMI_WS_' + frame.code, 'Kimi WebSocket command was rejected')); }
          return;
        }
        this.receive(frame);
      } catch (cause) { this.failure = cause; }
    });
    this.socket.addEventListener('close', () => {
      this.closed = true;
      for (const waiter of this.pending.values()) { clearTimeout(waiter.timer); waiter.reject(error('WS_CLOSED', 'The owned Kimi socket closed')); }
      this.pending.clear();
    });
    this.socket.addEventListener('error', () => { this.failure = error('WS_ERROR', 'The authenticated Kimi socket failed'); });
    const deadline = Date.now() + 5000;
    while (this.socket.readyState !== WebSocket.OPEN) {
      checkAbort(signal);
      if (this.failure || this.closed) throw this.failure || error('WS_CLOSED', 'Kimi socket closed before admission');
      if (Date.now() > deadline) throw error('WS_TIMEOUT', 'The authenticated Kimi socket did not open');
      await pause(20);
    }
  }
  command(type, fields = {}, timeout = 3000) {
    if (this.socket?.readyState !== WebSocket.OPEN) return Promise.reject(error('WS_CLOSED', 'Kimi socket is unavailable'));
    const request_id = `flywheel-${++this.sequence}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(request_id); reject(error('WS_TIMEOUT', 'Kimi did not acknowledge ' + type)); }, timeout);
      this.pending.set(request_id, { resolve, reject, timer });
      this.socket.send(JSON.stringify({ type, request_id, ...fields }));
    });
  }
  check() {
    if (this.failure) throw this.failure;
    if (this.closed || Date.now() - this.lastMessage > 25_000) throw error('WS_LOST', 'Kimi conversation continuity was lost; the prompt will not be retried');
  }
  close() { this.socket?.close(); }
}
