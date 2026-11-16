#!/usr/bin/env node
// A real HTTP/WebSocket CLI fixture implementing the checked upstream contract.
// It never contacts a model or an account; unknowns are exercised explicitly.
import http from 'node:http';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile, appendFile, rm } from 'node:fs/promises';
import { join } from 'node:path';

if (process.argv.includes('--version')) { process.stdout.write('kimi 9.8.7-fixture\n'); process.exit(0); }
if (process.argv.includes('-p')) {
  const args = process.argv.slice(2), id = args[args.indexOf('--session') + 1];
  const sessions = JSON.parse(await readFile(join(process.env.KIMI_CODE_HOME, 'fixture-sessions.json'), 'utf8'));
  if (!sessions[id]?.title || sessions[id].agent_config?.permission_mode !== 'auto' || sessions[id].agent_config?.plan_mode !== false) process.exit(92);
  if (process.env.KIMI_FIXTURE_LOG) await appendFile(process.env.KIMI_FIXTURE_LOG,
    JSON.stringify({ type: 'print', sessionId: id, cwd: process.cwd(), args }) + '\n');
  process.stdout.write(JSON.stringify({ role: 'assistant', content: 'Named print fixture completed' }) + '\n');
  process.stdout.write(JSON.stringify({ role: 'meta', type: 'session.resume_hint', session_id: id }) + '\n');
  process.exit(0);
}
if (JSON.stringify(process.argv.slice(2)) !== JSON.stringify(['web', '--no-open', '--host', '127.0.0.1', '--port', '0'])) process.exit(2);
const fixtureCase = process.env.KIMI_FIXTURE_CASE || 'normal';
const root = process.env.KIMI_CODE_HOME;
const token = 'fixture-secret-must-never-appear';
const registry = join(root, 'server', 'instances');
const registryPath = join(registry, `owned-${process.pid}.json`);
await mkdir(registry, { recursive: true });
await writeFile(join(root, 'server.token'), token);
const log = async value => { if (process.env.KIMI_FIXTURE_LOG) await appendFile(process.env.KIMI_FIXTURE_LOG, JSON.stringify(value) + '\n'); };
let sessions = {};
try { sessions = JSON.parse(await readFile(join(root, 'fixture-sessions.json'), 'utf8')); } catch { /* Fresh runtime. */ }
const save = () => writeFile(join(root, 'fixture-sessions.json'), JSON.stringify(sessions));
const sockets = new Set();
const paths = ['/api/v1/meta', '/api/v1/sessions', '/api/v1/sessions/{session_id}', '/api/v1/sessions/{session_id}/profile',
  '/api/v1/sessions/{session_id}/environment', '/api/v1/sessions/{session_id}/prompts', '/api/v1/sessions/{session_id}/agents/{agent_id}/turns',
  '/api/v1/shutdown', '/api/v1/remote-control'];
const property = { type: 'string' };
const schema = properties => ({ type: 'object', properties });
const config = schema(Object.fromEntries(['model', 'thinking', 'permission_mode', 'plan_mode', 'swarm_mode', 'profile', 'tower_mode', 'tower_base'].map(key => [key, property])));
const requestBody = properties => ({ requestBody: { content: { 'application/json': { schema: schema(properties) } } } });
const openapi = { openapi: '3.1.0', paths: Object.fromEntries(paths.map(path => [path, { get: {}, post: {} }])) };
openapi.paths['/api/v1/sessions'].post = requestBody({ title: property, metadata: schema({ cwd: property }), agent_config: config, environment_id: property, environment_cwd: property });
if (fixtureCase === 'no-title-schema') delete openapi.paths['/api/v1/sessions'].post.requestBody.content['application/json'].schema.properties.title;
openapi.paths['/api/v1/sessions/{session_id}/profile'].post = requestBody({ agent_config: config });
openapi.paths['/api/v1/sessions/{session_id}/prompts'].post = requestBody({ content: property, prompt_id: property, disabled_tools: property });
if (fixtureCase === 'missing-schema') delete openapi.paths['/api/v1/sessions/{session_id}/prompts'].post.requestBody.content['application/json'].schema.properties.disabled_tools;
const names = ['subscribe', 'response', 'agent', 'task', 'task.list', 'task.stop', 'turns', 'turn.started', 'turn.ended', 'interaction', 'ping', 'pong'];
const asyncapi = { asyncapi: '3.1.0', channels: { kimiCodeWebSocket: { address: '/api/v1/ws' } }, components: { messages: Object.fromEntries(names.map(name => [name.replaceAll('.', '_'), { name }])) } };
const agentState = (s, agentId = 'main') => ({ type: 'agent', session_id: s.id, agent_id: agentId,
  status: s.active ? 'running' : 'idle', compacting: false, model: s.agent_config.model, thinking: s.agent_config.thinking,
  permission_mode: fixtureCase === 'mismatch-permission' ? 'manual' : 'yolo', plan_mode: false,
  swarm_mode: Boolean(s.agent_config.swarm_mode), tower_mode: Boolean(s.agent_config.tower_mode) });
const sessionState = s => ({ ...s, busy: Boolean(s.active || s.task?.status === 'running'), main_turn_active: Boolean(s.active),
  pending_interaction: s.question ? 'question' : 'none', usage: { input_tokens: 0, output_tokens: 0, total_cost_usd: 0 } });
function frame(socket, value) {
  const payload = Buffer.from(JSON.stringify(value));
  let header;
  if (payload.length < 126) header = Buffer.from([0x81, payload.length]);
  else if (payload.length < 65536) { header = Buffer.alloc(4); header[0] = 0x81; header[1] = 126; header.writeUInt16BE(payload.length, 2); }
  else { header = Buffer.alloc(10); header[0] = 0x81; header[1] = 127; header.writeBigUInt64BE(BigInt(payload.length), 2); }
  socket.write(Buffer.concat([header, payload]));
}
function publish(s, value) { for (const socket of sockets) if (socket.subscriptions?.get(s.id)?.has(value.agent_id)) frame(socket, { session_id: s.id, ...value }); }
function terminal(s, outcome = 'completed') {
  if (!s.active) return;
  const turn = s.turns.at(-1);
  turn.status = 'ended'; turn.outcome = outcome;
  if (fixtureCase !== 'missing-usage') turn.steps = [{ index: 1, outcome: 'completed', usage: { input_other: 11, output: 7, input_cache_read: 3, input_cache_creation: 2 } }];
  turn.items.push({ type: 'assistant_message', item_id: 'assistant-' + turn.ordinal, text: 'Verified Kimi fixture result ' + (turn.ordinal + 1) });
  s.active = null; s.question = false;
  publish(s, { type: 'item.ended', agent_id: 'main', turn_id: turn.turn_id, item: turn.items.at(-1) });
  publish(s, { type: 'turn.ended', agent_id: 'main', turn: { ...turn, items: undefined } });
  publish(s, agentState(s));
  void save();
}
const server = http.createServer(async (req, res) => {
  try {
    if (req.headers.authorization !== 'Bearer ' + token) { res.writeHead(401); res.end('{}'); return; }
    const path = new URL(req.url, 'http://localhost').pathname;
    let body;
    if (req.method === 'POST') { let data = ''; for await (const chunk of req) data += chunk; body = data ? JSON.parse(data) : {}; }
    await log({ method: req.method, path, ...(body ? { body } : {}) });
    const send = (data, code = 0) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify({ code, data })); };
    if (path === '/openapi.json' || path === '/asyncapi.json') { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(path === '/openapi.json' ? openapi : asyncapi)); return; }
    if (path === '/api/v1/meta') { send({ server_id: 'owned-' + process.pid, server_version: 'fixture-9.8.7', backend: 'v2', dangerous_bypass_auth: false, capabilities: { websocket: true } }); return; }
    if (path === '/api/v1/remote-control') { send({ state: 'off' }); return; }
    if (path === '/api/v1/shutdown') {
      send({ ok: true });
      await save();
      if (fixtureCase === 'ignores-shutdown') return;
      setTimeout(async () => { for (const socket of sockets) socket.destroy(); await rm(registryPath, { force: true }); server.close(() => process.exit(0)); }, 10);
      return;
    }
    if (path === '/api/v1/sessions' && req.method === 'POST') {
      const id = 'session-' + (Object.keys(sessions).length + 1);
      const s = sessions[id] = { id, title: fixtureCase === 'wrong-title' ? 'Session placeholder' : body.title,
        metadata: body.metadata, agent_config: body.agent_config, environment_id: body.environment_id,
        cwd: body.environment_cwd || body.metadata.cwd, active: null, queued: [], turns: [] };
      await save(); send(sessionState(s)); return;
    }
    const match = path.match(/^\/api\/v1\/sessions\/([^/:]+)(.*)$/);
    const s = match && sessions[decodeURIComponent(match[1])];
    if (!s) { send(null, 40401); return; }
    const suffix = match[2];
    if (suffix === '' || suffix === '/profile') {
      if (body) { Object.assign(s.agent_config, body.agent_config); await save(); }
      send(sessionState(s)); return;
    }
    if (suffix === '/environment') { send({ environment_id: fixtureCase === 'remote-environment' ? 'ssh-fixture' : 'local', cwd: s.cwd }); return; }
    if (suffix === ':abort') {
      if (fixtureCase !== 'unstoppable') terminal(s, 'interrupted');
      send({ aborted: true }); return;
    }
    if (suffix.startsWith('/agents/')) {
      const agentId = decodeURIComponent(suffix.split('/')[2]);
      send({ turns: agentId === 'main' ? s.turns : s.childTurns || [], has_more: false }); return;
    }
    if (suffix === '/prompts' && req.method === 'GET') { send({ active: s.active, queued: s.queued }); return; }
    if (/^\/prompts\/.+:abort$/.test(suffix)) { s.queued = []; send({ aborted: true }); return; }
    if (suffix === '/prompts' && req.method === 'POST') {
      const ordinal = s.turns.length;
      s.active = { prompt_id: body.prompt_id, user_message_id: 'user-' + ordinal, status: 'running' };
      const user = { type: 'user_message', item_id: 'user-' + ordinal, prompt_ids: [body.prompt_id] };
      const turn = { turn_id: 'turn-' + (ordinal + 1), ordinal, kind: 'agent', status: 'running', items: [user], steps: [] };
      s.turns.push(turn);
      publish(s, agentState(s));
      publish(s, { type: 'turn.started', agent_id: 'main', turn: { ...turn, items: undefined } });
      publish(s, { type: 'item.ended', agent_id: 'main', turn_id: turn.turn_id, item: user });
      if (['child', 'child-progress', 'cancel-queue'].includes(fixtureCase)) {
        s.task = { type: 'task', agent_id: 'main', task_id: 'task-child', kind: 'subagent', child_agent_id: 'child-1', status: 'running', model: 'native-child-model' };
        s.childTurns = [{ turn_id: 'child-turn', ordinal: 0, kind: 'agent', status: 'running', items: [], steps: [] }];
        publish(s, s.task);
      }
      if (fixtureCase === 'cancel-queue') s.queued = [{ prompt_id: 'queued-future', status: 'queued' }];
      if (fixtureCase === 'question') { s.question = true; publish(s, { type: 'interaction', agent_id: 'main', interaction_id: 'question-1', status: 'pending', kind: 'question' }); }
      if (s.agent_config.tower_mode) await writeFile(join(s.cwd, 'tower-result.txt'), 'native Tower candidate\n');
      const accepted = s.active;
      // Events precede HTTP acceptance to exercise cross-channel correlation.
      setTimeout(() => send(accepted), 5);
      const streamsProgress = ['root-progress', 'child-progress', 'burst-progress'].includes(fixtureCase);
      if (streamsProgress) {
        const agentId = fixtureCase === 'child-progress' ? 'child-1' : 'main';
        const activeTurn = agentId === 'main' ? turn : s.childTurns[0];
        const item = { type: 'tool_call', name: 'Read', item_id: 'progress-item', output: '' };
        activeTurn.items.push(item);
        publish(s, { type: 'item.started', agent_id: agentId, turn_id: activeTurn.turn_id, item });
        const timer = setInterval(() => {
          if (!s.active) { clearInterval(timer); return; }
          const delta = 'private-tool-progress';
          for (let index = 0; index < (fixtureCase === 'burst-progress' ? 250 : 1); index++) {
            item.output += delta;
            publish(s, { type: 'item.delta', agent_id: agentId, turn_id: activeTurn.turn_id, item_id: item.item_id, field: 'output', delta });
          }
          publish(s, { type: 'item.updated', agent_id: agentId, turn_id: activeTurn.turn_id, item_id: item.item_id, set: { status: 'running' } });
          publish(s, { type: 'turn.updated', agent_id: agentId, turn_id: activeTurn.turn_id, step: { index: 1, status: 'running' } });
        }, 80);
      }
      if (!['cancel-queue', 'question', 'unstoppable'].includes(fixtureCase)) setTimeout(() => {
        if (s.task) {
          s.task = { ...s.task, status: 'ended', outcome: 'completed' }; publish(s, s.task);
          s.childTurns[0] = { ...s.childTurns[0], status: 'ended', outcome: 'completed', steps: [{ index: 1, usage: { input_other: 5, output: 2, input_cache_read: 0, input_cache_creation: 0 } }] };
          publish(s, { type: 'turn.ended', agent_id: 'child-1', turn: s.childTurns[0] });
          publish(s, { ...agentState(s, 'child-1'), status: 'idle' });
        }
        terminal(s);
      }, streamsProgress ? 3000 : 160);
      return;
    }
    send(null, 40400);
  } catch { res.writeHead(500); res.end('{}'); }
});
server.on('upgrade', (req, socket) => {
  const protocol = 'kimi-code.bearer.' + token;
  if (req.headers['sec-websocket-protocol'] !== protocol) { socket.destroy(); return; }
  const accept = createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
  socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\nSec-WebSocket-Protocol: ${protocol}\r\n\r\n`);
  sockets.add(socket); socket.subscriptions = new Map();
  socket.on('close', () => sockets.delete(socket)); socket.on('error', () => sockets.delete(socket));
  let buffer = Buffer.alloc(0);
  socket.on('data', async data => {
    buffer = Buffer.concat([buffer, data]);
    while (buffer.length >= 2) {
      const opcode = buffer[0] & 15; let length = buffer[1] & 127, offset = 2;
      if (length === 126) { if (buffer.length < 4) return; length = buffer.readUInt16BE(2); offset = 4; }
      if (length === 127) { if (buffer.length < 10) return; length = Number(buffer.readBigUInt64BE(2)); offset = 10; }
      const masked = Boolean(buffer[1] & 128), mask = masked ? buffer.subarray(offset, offset + 4) : undefined;
      if (masked) offset += 4;
      if (buffer.length < offset + length) return;
      const payload = Buffer.from(buffer.subarray(offset, offset + length)); buffer = buffer.subarray(offset + length);
      if (mask) for (let index = 0; index < payload.length; index++) payload[index] ^= mask[index % 4];
      if (opcode === 8) { socket.end(Buffer.from([0x88, 0])); return; }
      if (opcode !== 1) continue;
      let command; try { command = JSON.parse(payload.toString()); } catch { socket.destroy(); return; }
      await log({ websocket: command.type, ...command });
      if (command.type === 'pong') continue;
      const s = sessions[command.session_id];
      frame(socket, { type: 'response', request_id: command.request_id, code: s ? 0 : 40401, ...(command.type === 'task.stop' && s?.task ? { task: { ...s.task, status: 'ended', outcome: 'cancelled' } } : {}) });
      if (!s) continue;
      if (command.type === 'subscribe') {
        let ids = socket.subscriptions.get(s.id); if (!ids) socket.subscriptions.set(s.id, ids = new Set());
        for (const id of command.agent_ids) {
          if (ids.has(id)) continue; ids.add(id);
          frame(socket, { type: 'task.list', session_id: s.id, agent_id: id, tasks: id === 'main' && s.task?.status === 'running' ? [s.task] : [] });
          frame(socket, agentState(s, id));
          frame(socket, { type: 'turns', session_id: s.id, agent_id: id, turns: id === 'main' ? s.turns : s.childTurns || [], has_more: false });
        }
      }
      if (command.type === 'task.stop' && s.task) {
        s.task = { ...s.task, status: 'ended', outcome: 'cancelled' };
        s.childTurns[0] = { ...s.childTurns[0], status: 'ended', outcome: 'interrupted' };
        publish(s, s.task); publish(s, { ...agentState(s, 'child-1'), status: 'idle' });
      }
    }
  });
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
await writeFile(registryPath, JSON.stringify({ server_id: 'owned-' + process.pid, pid: process.pid, host: '127.0.0.1', port: server.address().port, started_at: Date.now(), heartbeat_at: Date.now() }));
process.stdout.write(`Local: http://127.0.0.1:${server.address().port}/#token=${token}\n`);
process.stderr.write('Token: ' + token + '\n');
if (fixtureCase === 'unstoppable') process.on('SIGTERM', () => {});
