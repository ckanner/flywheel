import { spawn } from 'node:child_process';

export const CATALOG_BYTES = 1024 * 1024;
export class CatalogError extends Error {
  constructor(code) { super(code); this.code = code; }
}

/** Metadata children remain in the Go watchdog's process group. No shell,
 * detached daemon, user task, tool call or login command is admitted here. */
export class CatalogProcess {
  constructor(command, args, { cwd, env, signal, timeoutMs = 10000, spawnProcess = spawn } = {}) {
    this.pending = new Map();
    this.bytes = 0;
    this.buffer = '';
    this.output = '';
    this.nextID = 0;
    this.decoder = new TextDecoder('utf-8', { fatal: true });
    this.failure = null;
    this.stopping = false;
    this.signal = signal;
    this.child = spawnProcess(command, args, { cwd, env, shell: false, detached: false, windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'] });
    this.exited = new Promise(resolve => { this.resolveExit = resolve; });
    this.child.stdin.on('error', () => this.fail('CATALOG_UNAVAILABLE'));
    this.child.on('error', () => { this.fail('CATALOG_UNAVAILABLE'); });
    this.child.on('close', (code, signal) => {
      this.closed = true;
      try { this.decoder.decode(); } catch { this.fail('CATALOG_PROTOCOL'); }
      if (!this.stopping && (code !== 0 || signal)) this.fail('CATALOG_UNAVAILABLE');
      for (const { reject } of this.pending.values()) reject(new CatalogError(this.failure ?? 'CATALOG_PROTOCOL'));
      this.pending.clear();
      this.resolveExit();
    });
    this.child.stdout.on('data', chunk => this.accept(chunk));
    // Diagnostics may contain paths or secrets. Count but never retain them.
    this.child.stderr.on('data', chunk => this.count(chunk));
    this.abort = () => this.fail('CANCELLED');
    signal?.addEventListener('abort', this.abort, { once: true });
    this.timer = setTimeout(() => this.fail('CATALOG_TIMEOUT'), timeoutMs);
    if (signal?.aborted) this.abort();
  }

  count(chunk) {
    this.bytes += chunk.length;
    if (this.bytes > CATALOG_BYTES) this.fail('OUTPUT_LIMIT');
    return !this.failure;
  }

  fail(code) {
    this.failure ??= code;
    for (const { reject } of this.pending.values()) reject(new CatalogError(this.failure));
    this.pending.clear();
    if (!this.closed) this.child.kill('SIGKILL');
  }

  accept(chunk) {
    if (!this.count(chunk)) return;
    let decoded;
    try { decoded = this.decoder.decode(chunk, { stream: true }); } catch { this.fail('CATALOG_PROTOCOL'); return; }
    if (!this.rpc) { this.output += decoded; return; }
    this.buffer += decoded;
    let newline;
    while ((newline = this.buffer.indexOf('\n')) >= 0) {
      const line = this.buffer.slice(0, newline);
      this.buffer = this.buffer.slice(newline + 1);
      if (!line.trim()) continue;
      let frame;
      try { frame = JSON.parse(line); } catch { this.fail('CATALOG_PROTOCOL'); return; }
      if (!frame || typeof frame !== 'object' || Array.isArray(frame) || typeof frame.method === 'string' && frame.id !== undefined) {
        this.fail('CATALOG_PROTOCOL'); return;
      }
      if (frame.id === undefined) continue; // metadata notifications carry no model rows
      const pending = this.pending.get(frame.id);
      if (!pending) { this.fail('CATALOG_PROTOCOL'); return; }
      this.pending.delete(frame.id);
      if (frame.error || !Object.hasOwn(frame, 'result')) pending.reject(new CatalogError('CATALOG_PROTOCOL'));
      else pending.resolve(frame.result);
    }
  }

  request(method, params) {
    this.rpc = true;
    if (this.failure || this.closed) return Promise.reject(new CatalogError(this.failure ?? 'CATALOG_UNAVAILABLE'));
    const id = ++this.nextID;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
    });
  }

  notify(method) {
    if (this.failure || this.closed) throw new CatalogError(this.failure ?? 'CATALOG_UNAVAILABLE');
    this.child.stdin.write(JSON.stringify({ method }) + '\n');
  }

  async text() {
    this.child.stdin.end();
    await this.exited;
    if (this.failure) throw new CatalogError(this.failure);
    return this.output;
  }

  async close() {
    this.stopping = true;
    clearTimeout(this.timer);
    this.signal?.removeEventListener('abort', this.abort);
    if (!this.closed) {
      this.child.stdin.end();
      this.child.kill('SIGTERM');
      const timer = setTimeout(() => this.child.kill('SIGKILL'), 500);
      try { await this.exited; } finally { clearTimeout(timer); }
    }
  }
}
