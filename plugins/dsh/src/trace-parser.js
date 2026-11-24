import { readdir, readFile, realpath, stat } from 'node:fs/promises';
import { isAbsolute, join, sep } from 'node:path';

const MAX_SESSION_FILE_BYTES = 8 * 1024 * 1024;
const MAX_BLOCKS = 240;
const MAX_BLOCK_CHARS = 12000;
const MAX_RAW_TAIL_LINES = 80;
const SAFE_SESSION_ID = /^[A-Za-z0-9._:@/-]{1,200}$/;

function insideDir(parent, child) {
  return child === parent || child.startsWith(parent + sep);
}

export function shellQuote(value) {
  if (typeof value !== 'string' || value.includes('\0') || value.includes('\n') || value.includes('\r')) {
    return '';
  }
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

function truncateText(text, limit = MAX_BLOCK_CHARS) {
  if (typeof text !== 'string') return '';
  if (text.length <= limit) return text;
  return text.slice(0, limit) + '\n… [truncated]';
}

function stringifyCompact(value, limit = 2000) {
  if (value === undefined || value === null) return '';
  if (typeof value === 'string') return truncateText(value, limit);
  try {
    return truncateText(JSON.stringify(value, null, 2), limit);
  } catch {
    return truncateText(String(value), limit);
  }
}

export function buildResumeInfo(harness, sessionId, cwd) {
  const id = typeof sessionId === 'string' ? sessionId.trim() : '';
  if (!id || !SAFE_SESSION_ID.test(id)) return null;
  const h = typeof harness === 'string' ? harness.trim().toLowerCase() : '';
  const hasCwd = typeof cwd === 'string' && isAbsolute(cwd) && !cwd.includes('\0') && !cwd.includes('\n') && !cwd.includes('\r');
  const cdPrefix = hasCwd ? `cd ${shellQuote(cwd)} && ` : '';
  const qId = shellQuote(id);

  if (h === 'dsh') {
    return {
      kind: 'dsh-native',
      harness: 'dsh',
      providerLabel: 'DSH Desktop',
      sessionId: id,
      cwd: hasCwd ? cwd : '',
      command: '',
    };
  }
  if (h === 'claude-code-pro') {
    return {
      kind: 'cli',
      harness: 'claude-code-pro',
      providerLabel: 'Claude Code',
      sessionId: id,
      cwd: hasCwd ? cwd : '',
      command: `${cdPrefix}claude --resume ${qId}`,
    };
  }
  if (h === 'codex-pro' || h === 'codex') {
    return {
      kind: 'cli',
      harness: h,
      providerLabel: 'Codex',
      sessionId: id,
      cwd: hasCwd ? cwd : '',
      command: `${cdPrefix}codex resume ${qId}`,
    };
  }
  if (h === 'kimi') {
    return {
      kind: 'cli',
      harness: 'kimi',
      providerLabel: 'Kimi CLI',
      sessionId: id,
      cwd: hasCwd ? cwd : '',
      command: `${cdPrefix}kimi --session ${qId}`,
      secondaryCommand: `${cdPrefix}kimi vis ${qId}`,
    };
  }
  if (h === 'antigravity') {
    return {
      kind: 'cli',
      harness: 'antigravity',
      providerLabel: 'Antigravity CLI',
      sessionId: id,
      cwd: hasCwd ? cwd : '',
      command: `${cdPrefix}agy --conversation ${qId}`,
    };
  }
  return null;
}

function pushBlock(blocks, block) {
  if (!block || blocks.length >= MAX_BLOCKS) return;
  const text = typeof block.text === 'string' ? block.text : '';
  const prev = blocks[blocks.length - 1];
  // Coalesce consecutive thinking or assistant chunks into readable blocks.
  if (
    prev &&
    prev.kind === block.kind &&
    (block.kind === 'thinking' || block.kind === 'assistant') &&
    prev.title === block.title
  ) {
    const sepStr = prev.text.endsWith('\n') || text.startsWith('\n') || text.length < 40 ? '' : '\n';
    prev.text = truncateText(prev.text + sepStr + text, MAX_BLOCK_CHARS);
    if (block.ts) prev.ts = block.ts;
    return;
  }
  // Deduplicate identical back-to-back progress lines.
  if (prev && prev.kind === 'progress' && block.kind === 'progress' && prev.text === text) {
    if (block.ts) prev.ts = block.ts;
    return;
  }
  blocks.push({
    ...block,
    text: truncateText(text, MAX_BLOCK_CHARS),
  });
}

function extractContentParts(content) {
  if (typeof content === 'string') return [{ type: 'text', text: content }];
  if (!Array.isArray(content)) return [];
  return content.filter(part => part && typeof part === 'object');
}

function parseJsonLine(rawLine, ts, blocks, sessionRef) {
  let obj;
  try {
    obj = JSON.parse(rawLine);
  } catch {
    return;
  }
  if (!obj || typeof obj !== 'object') return;

  // Capture any session / thread ID emitted in stream frames.
  const discoveredSession =
    (typeof obj.sessionId === 'string' && obj.sessionId) ||
    (typeof obj.session_id === 'string' && obj.session_id) ||
    (typeof obj.thread_id === 'string' && obj.thread_id) ||
    (typeof obj.threadId === 'string' && obj.threadId) ||
    (typeof obj.receipt?.sessionId === 'string' && obj.receipt.sessionId) ||
    (typeof obj.receipt?.threadId === 'string' && obj.receipt.threadId) ||
    (typeof obj.receipt?.conversationId === 'string' && obj.receipt.conversationId) ||
    (typeof obj.receipt?.outcome?.sessionId === 'string' && obj.receipt.outcome.sessionId) ||
    '';
  if (discoveredSession && !sessionRef.sessionId) {
    sessionRef.sessionId = discoveredSession;
  }

  // 1. Kimi stream-json (`role: "assistant" | "tool"`)
  if (obj.role === 'assistant') {
    if (typeof obj.content === 'string' && obj.content.trim()) {
      pushBlock(blocks, { kind: 'assistant', ts, text: obj.content });
    } else if (Array.isArray(obj.content)) {
      for (const part of obj.content) {
        if (!part || typeof part !== 'object') continue;
        if ((part.type === 'think' || part.type === 'thinking') && (part.think || part.thinking || part.text)) {
          pushBlock(blocks, { kind: 'thinking', ts, text: String(part.think ?? part.thinking ?? part.text) });
        } else if (part.type === 'text' && part.text) {
          pushBlock(blocks, { kind: 'assistant', ts, text: String(part.text) });
        }
      }
    }
    if (Array.isArray(obj.tool_calls)) {
      for (const call of obj.tool_calls) {
        const name = call?.function?.name || call?.name || 'tool';
        const args = call?.function?.arguments ?? call?.arguments ?? call?.input;
        pushBlock(blocks, { kind: 'tool_call', ts, title: String(name), text: stringifyCompact(args, 2400) });
      }
    }
    return;
  }

  if (obj.role === 'tool') {
    const content = typeof obj.content === 'string' ? obj.content : stringifyCompact(obj.content, 2400);
    pushBlock(blocks, { kind: 'tool_result', ts, title: obj.name || obj.tool_call_id || 'tool', text: content });
    return;
  }

  // 2. Claude SDK stream-json (`type: "assistant" | "user"`)
  if (obj.type === 'assistant' && Array.isArray(obj.message?.content)) {
    for (const part of obj.message.content) {
      if (!part || typeof part !== 'object') continue;
      if (part.type === 'thinking' && (part.thinking || part.text)) {
        pushBlock(blocks, { kind: 'thinking', ts, text: String(part.thinking ?? part.text) });
      } else if (part.type === 'text' && part.text) {
        pushBlock(blocks, { kind: 'assistant', ts, text: String(part.text) });
      } else if (part.type === 'tool_use') {
        pushBlock(blocks, { kind: 'tool_call', ts, title: String(part.name || 'tool'), text: stringifyCompact(part.input, 2400) });
      }
    }
    return;
  }

  if (obj.type === 'user' && Array.isArray(obj.message?.content)) {
    for (const part of obj.message.content) {
      if (part?.type === 'tool_result') {
        pushBlock(blocks, {
          kind: part.is_error ? 'error' : 'tool_result',
          ts,
          title: part.tool_use_id || 'tool_result',
          text: stringifyCompact(part.content, 2400),
        });
      }
    }
    return;
  }

  // 3. Antigravity ACP / native-event frames
  const acpUpdate = obj.event?.params?.update ?? obj.params?.update ?? (obj.sessionUpdate ? obj : null);
  if (acpUpdate && typeof acpUpdate.sessionUpdate === 'string') {
    const uKind = acpUpdate.sessionUpdate;
    const bodyText =
      (typeof acpUpdate.content?.text === 'string' && acpUpdate.content.text) ||
      (typeof acpUpdate.text === 'string' && acpUpdate.text) ||
      '';
    if (uKind === 'agent_thought_chunk' && bodyText) {
      pushBlock(blocks, { kind: 'thinking', ts, text: bodyText });
    } else if (uKind === 'agent_message_chunk' && bodyText) {
      pushBlock(blocks, { kind: 'assistant', ts, text: bodyText });
    } else if (uKind === 'tool_call' || uKind === 'tool_call_update') {
      const title = acpUpdate.title || acpUpdate.name || acpUpdate.toolCallId || 'tool';
      const detail = acpUpdate.rawOutput !== undefined
        ? stringifyCompact(acpUpdate.rawOutput, 2400)
        : stringifyCompact(acpUpdate.rawInput ?? acpUpdate.content, 2400);
      pushBlock(blocks, {
        kind: uKind === 'tool_call_update' && acpUpdate.status === 'completed' ? 'tool_result' : 'tool_call',
        ts,
        title: `${title}${acpUpdate.status ? ` (${acpUpdate.status})` : ''}`,
        text: detail,
      });
    }
    return;
  }

  // 4. Codex app-server / CLI items
  if ((obj.type === 'item.completed' || obj.type === 'item.started' || obj.type === 'item.updated') && obj.item) {
    const item = obj.item;
    if (item.type === 'reasoning' && (item.text || item.summary)) {
      pushBlock(blocks, { kind: 'thinking', ts, text: String(item.text ?? item.summary) });
    } else if (item.type === 'agent_message' && item.text) {
      pushBlock(blocks, { kind: 'assistant', ts, text: String(item.text) });
    } else if (item.type === 'command_execution') {
      const cmd = item.command || 'shell';
      const out = item.aggregated_output || '';
      pushBlock(blocks, {
        kind: obj.type === 'item.completed' ? 'tool_result' : 'tool_call',
        ts,
        title: `shell: ${truncateText(String(cmd), 120)}`,
        text: out || (item.exit_code !== undefined ? `exit ${item.exit_code}` : ''),
      });
    } else if (item.type === 'mcp_tool_call') {
      pushBlock(blocks, {
        kind: obj.type === 'item.completed' ? 'tool_result' : 'tool_call',
        ts,
        title: String(item.tool || item.name || 'mcp'),
        text: stringifyCompact(item.result ?? item.arguments, 2400),
      });
    }
    return;
  }

  // 5. DSH / Worker envelopes (`progress`, `activity`, `event`, `thinking`, `error`, `result`)
  if (obj.type === 'thinking' && (obj.text || obj.thinking)) {
    pushBlock(blocks, { kind: 'thinking', ts, text: String(obj.text ?? obj.thinking) });
    return;
  }
  if (obj.type === 'assistant' && typeof obj.text === 'string' && obj.text) {
    pushBlock(blocks, { kind: 'assistant', ts, text: obj.text });
    return;
  }
  if (obj.type === 'progress' && typeof obj.text === 'string' && obj.text.trim()) {
    pushBlock(blocks, { kind: 'progress', ts, text: obj.text.trim() });
    return;
  }
  if (obj.type === 'activity' && obj.activity) {
    const act = obj.activity;
    const parts = [act.phase, act.summary, act.detail, act.tool, act.target].filter(v => typeof v === 'string' && v.trim());
    if (parts.length > 0) {
      pushBlock(blocks, { kind: 'progress', ts, title: act.source || 'activity', text: parts.join(' · ') });
    }
    return;
  }
  if (obj.type === 'event' && obj.event && typeof obj.event === 'object') {
    const ev = obj.event;
    const evType = String(ev.type || '');
    const data = ev.data && typeof ev.data === 'object' ? ev.data : ev;
    if (evType === 'user/message' || evType === 'agent/inbox/spliced' || evType === 'request/header') {
      return;
    }
    if (evType === 'assistant/message') {
      const parts = extractContentParts(data.message?.content ?? data.content);
      for (const part of parts) {
        if ((part.type === 'thinking' || part.type === 'reasoning') && (part.thinking || part.reasoning || part.text)) {
          pushBlock(blocks, { kind: 'thinking', ts, text: String(part.thinking ?? part.reasoning ?? part.text) });
        } else if (part.type === 'text' && part.text) {
          pushBlock(blocks, { kind: 'assistant', ts, text: String(part.text) });
        }
      }
      return;
    }
    if (evType === 'assistant/attempt' && Array.isArray(data.stream)) {
      for (const entry of data.stream) {
        const failure = entry?.chunk?.reason?.failure;
        if (failure?.message) {
          pushBlock(blocks, { kind: 'error', ts, title: failure.code || 'attempt-error', text: String(failure.message) });
        }
      }
      return;
    }
    if (evType.includes('thinking') || evType.includes('reasoning')) {
      const text = data.delta || data.text || data.reasoning || data.thinking || '';
      if (typeof text === 'string' && text) pushBlock(blocks, { kind: 'thinking', ts, text });
    } else if (evType === 'tool/call' || evType === 'tool_call') {
      pushBlock(blocks, { kind: 'tool_call', ts, title: String(data.name || 'tool'), text: stringifyCompact(data.arguments ?? data.input, 2400) });
    } else if (evType === 'tool/result' || evType === 'tool_result') {
      const parts = extractContentParts(data.message?.content ?? data.content);
      const text = parts.length > 0
        ? parts.map(p => (typeof p.text === 'string' ? p.text : stringifyCompact(p, 1200))).join('\n')
        : stringifyCompact(data.result ?? data.output ?? data.message, 2400);
      pushBlock(blocks, { kind: 'tool_result', ts, title: String(data.name || data.message?.toolCallId || 'tool'), text });
    } else if (evType.startsWith('assistant/') && typeof (data.delta || data.text) === 'string') {
      const text = data.delta || data.text;
      if (text) pushBlock(blocks, { kind: 'assistant', ts, text });
    }
    return;
  }
  if (obj.type === 'error' || obj.type === 'turn.failed') {
    const msg = obj.error?.message || obj.message || stringifyCompact(obj.error ?? obj, 1200);
    pushBlock(blocks, { kind: 'error', ts, title: obj.error?.code || 'error', text: String(msg) });
    return;
  }
  if (obj.type === 'result') {
    const failMsg = obj.error?.message || obj.receipt?.failure?.message;
    if (failMsg) {
      pushBlock(blocks, { kind: 'error', ts, title: obj.error?.code || 'failed', text: String(failMsg) });
    }
    const finalOut =
      (typeof obj.text === 'string' && obj.text) ||
      (Array.isArray(obj.receipt?.outcome?.output) && obj.receipt.outcome.output.map(o => o?.text || '').join('\n')) ||
      '';
    if (finalOut.trim() && !blocks.some(b => b.kind === 'assistant' && b.text.includes(finalOut.trim().slice(0, 80)))) {
      pushBlock(blocks, { kind: 'assistant', ts, text: finalOut });
    }
  }
}

function splitRoleScope(key) {
  if (typeof key !== 'string' || !key) return { role: 'unknown', scope: '' };
  const idx = key.indexOf('/');
  if (idx < 0) return { role: key, scope: '' };
  return { role: key.slice(0, idx), scope: key.slice(idx + 1) };
}

function inferStatus(info) {
  if (info.running) return 'running';
  if (info.cancelled) return 'cancelled';
  if (info.timedOut || info.errorCode || info.errorMessage) return 'failed';
  if (typeof info.exitCode === 'number' && info.exitCode !== 0) return 'failed';
  if (info.signal) return 'failed';
  if (info.stateStatus === 'blocked') return 'blocked';
  return 'completed';
}

function latestTracePreview(blocks) {
  if (!Array.isArray(blocks) || blocks.length === 0) return '';
  for (let i = blocks.length - 1; i >= 0; i -= 1) {
    const b = blocks[i];
    if (!b || !b.text) continue;
    const prefix = b.kind === 'tool_call' && b.title ? `[${b.title}] ` : '';
    const line = String(b.text).trim().replace(/\s+/g, ' ');
    if (line) return truncateText(prefix + line, 220);
  }
  return '';
}

function parseLiveJsonlFile(rawText, fileMtimeMs) {
  const lines = rawText.split('\n').filter(Boolean);
  let start = null;
  let end = null;
  let truncated = false;
  const events = [];
  for (const raw of lines) {
    let frame;
    try {
      frame = JSON.parse(raw);
    } catch {
      continue;
    }
    if (!frame || typeof frame !== 'object') continue;
    if (frame.kind === 'start') start = frame;
    else if (frame.kind === 'end') end = frame;
    else if (frame.kind === 'truncated') truncated = true;
    else if (frame.kind === 'event' && typeof frame.line === 'string') {
      events.push({ ts: frame.ts || '', line: frame.line });
    }
  }
  if (!start) return null;

  const sessionRef = { sessionId: end?.sessionId || start.requestedThread || '' };
  const blocks = [];
  for (const ev of events) {
    parseJsonLine(ev.line, ev.ts, blocks, sessionRef);
  }
  if (end?.text && !blocks.some(b => b.kind === 'assistant' && b.text.trim() === String(end.text).trim())) {
    pushBlock(blocks, { kind: 'assistant', ts: end.ts || '', text: String(end.text) });
  }
  if (end?.stderr && String(end.stderr).trim()) {
    pushBlock(blocks, { kind: 'error', ts: end.ts || '', title: 'stderr', text: String(end.stderr) });
  }
  if (end?.errorMessage) {
    pushBlock(blocks, { kind: 'error', ts: end.ts || '', title: end.errorCode || 'error', text: String(end.errorMessage) });
  }

  const running = !end;
  const harness = end?.harness || start.harness || '';
  const model = end?.model || start.model || '';
  const cwd = start.cwd || start.workspaceCwd || '';
  const status = inferStatus({
    running,
    cancelled: end?.cancelled,
    timedOut: end?.timedOut,
    exitCode: end?.exitCode,
    signal: end?.signal,
    errorCode: end?.errorCode,
    errorMessage: end?.errorMessage,
  });

  return {
    summary: {
      id: start.invocationId,
      key: start.key || `${start.role}/${start.scope}`,
      role: start.role || splitRoleScope(start.key).role,
      scope: start.scope || splitRoleScope(start.key).scope,
      attempt: start.attempt ?? 0,
      harness,
      model,
      thinking: start.thinking || '',
      cwd,
      status,
      running,
      exitCode: end?.exitCode ?? null,
      signal: end?.signal || '',
      durationMs: end?.durationMs ?? null,
      startedAt: start.ts || '',
      updatedAt: end?.ts || events.at(-1)?.ts || new Date(fileMtimeMs).toISOString(),
      sessionId: sessionRef.sessionId || '',
      resume: buildResumeInfo(harness, sessionRef.sessionId, cwd),
      errorCode: end?.errorCode || '',
      errorSummary: end?.errorMessage || (status === 'failed' && end?.stderr ? truncateText(String(end.stderr), 220) : ''),
      usage: end?.usage || null,
      source: 'live',
      eventCount: events.length,
      preview: latestTracePreview(blocks),
    },
    detail: {
      blocks,
      stderr: end?.stderr || '',
      text: end?.text || '',
      truncated: truncated || Boolean(end?.truncated),
      rawTail: events.slice(-MAX_RAW_TAIL_LINES).map(e => e.line),
    },
  };
}

function parseDurableReceiptFile(rawText, fileName, fileMtimeMs, stateRecords) {
  let receipt;
  try {
    receipt = JSON.parse(rawText);
  } catch {
    return null;
  }
  if (!receipt || typeof receipt !== 'object' || !receipt.key) return null;
  const result = receipt.result && typeof receipt.result === 'object' ? receipt.result : {};
  const stateRec = stateRecords?.[receipt.key] ?? null;
  const { role, scope } = splitRoleScope(receipt.key);

  const sessionRef = {
    sessionId:
      result.sessionId ||
      result.session?.sessionId ||
      stateRec?.['thread-id'] ||
      '',
  };
  const blocks = [];
  const rawStdout = typeof result.stdout === 'string' ? result.stdout : '';
  const stdoutLines = rawStdout.split('\n').filter(Boolean);
  for (const line of stdoutLines) {
    parseJsonLine(line, '', blocks, sessionRef);
  }
  if (result.text && !blocks.some(b => b.kind === 'assistant' && b.text.trim() === String(result.text).trim())) {
    pushBlock(blocks, { kind: 'assistant', ts: '', text: String(result.text) });
  }
  if (result.stderr && String(result.stderr).trim()) {
    pushBlock(blocks, { kind: 'error', ts: '', title: 'stderr', text: String(result.stderr) });
  }
  if (result.errorMessage) {
    pushBlock(blocks, { kind: 'error', ts: '', title: result.errorCode || 'error', text: String(result.errorMessage) });
  }

  const harness = result.harness || stateRec?.configuration?.harness || '';
  const model = result.model || stateRec?.configuration?.model || '';
  const thinking = stateRec?.configuration?.thinking || '';
  const cwd = receipt.cwd || stateRec?.cwd || '';
  const id = receipt['invocation-id'] || fileName.replace(/\.json$/, '');
  const status = inferStatus({
    running: false,
    cancelled: result.cancelled,
    timedOut: result.timedOut,
    exitCode: result.exitCode,
    signal: result.signal,
    errorCode: result.errorCode,
    errorMessage: result.errorMessage,
    stateStatus: stateRec?.last?.status,
  });

  return {
    summary: {
      id,
      key: receipt.key,
      role,
      scope,
      attempt: stateRec?.last?.attempt ?? 0,
      harness,
      model,
      thinking,
      cwd,
      status,
      running: false,
      exitCode: result.exitCode ?? null,
      signal: result.signal || '',
      durationMs: result.durationMs ?? null,
      startedAt: stateRec?.last?.['started-at'] || '',
      updatedAt: new Date(fileMtimeMs).toISOString(),
      sessionId: sessionRef.sessionId || '',
      resume: buildResumeInfo(harness, sessionRef.sessionId, cwd),
      errorCode: result.errorCode || '',
      errorSummary: result.errorMessage || (status === 'failed' && result.stderr ? truncateText(String(result.stderr), 220) : ''),
      usage: result.usage || null,
      source: 'receipt',
      eventCount: stdoutLines.length,
      preview: latestTracePreview(blocks),
    },
    detail: {
      blocks,
      stderr: result.stderr || '',
      text: result.text || '',
      truncated: Boolean(result.truncated),
      rawTail: stdoutLines.slice(-MAX_RAW_TAIL_LINES),
    },
  };
}

async function readTaskStateRecords(resolvedTask) {
  try {
    const statePath = await realpath(join(resolvedTask, 'state.json'));
    if (!insideDir(resolvedTask, statePath)) return {};
    const parsed = JSON.parse(await readFile(statePath, 'utf8'));
    return parsed?.sessions?.records && typeof parsed.sessions.records === 'object'
      ? parsed.sessions.records
      : {};
  } catch {
    return {};
  }
}

async function collectTraces(resolvedTask, targetId) {
  const stateRecords = await readTaskStateRecords(resolvedTask);
  const byId = new Map();

  // First pass: live-*.jsonl in dedicated `.traces/` (and legacy `.sessions/` fallback)
  for (const subDir of ['.traces', '.sessions']) {
    const dirPath = await realpath(join(resolvedTask, subDir)).catch(() => null);
    if (!dirPath || !insideDir(resolvedTask, dirPath)) continue;
    const names = (await readdir(dirPath).catch(() => [])).sort();
    for (const name of names) {
      if (!/^live-[A-Za-z0-9._-]+\.jsonl$/.test(name)) continue;
      const filePath = await realpath(join(dirPath, name)).catch(() => null);
      if (!filePath || !insideDir(dirPath, filePath)) continue;
      const info = await stat(filePath).catch(() => null);
      if (!info || !info.isFile() || info.size > MAX_SESSION_FILE_BYTES) continue;
      const raw = await readFile(filePath, 'utf8').catch(() => '');
      const parsed = parseLiveJsonlFile(raw, info.mtimeMs);
      if (!parsed || byId.has(parsed.summary.id)) continue;
      byId.set(parsed.summary.id, parsed);
    }
  }

  // Second pass: durable <hash>.json receipts in `.sessions/` (covers older runs & enriches sessionId)
  const sessionsDir = await realpath(join(resolvedTask, '.sessions')).catch(() => null);
  if (sessionsDir && insideDir(resolvedTask, sessionsDir)) {
    const names = (await readdir(sessionsDir).catch(() => [])).sort();
    for (const name of names) {
      if (!/^[a-f0-9]{64}\.json$/.test(name)) continue;
      const filePath = await realpath(join(sessionsDir, name)).catch(() => null);
      if (!filePath || !insideDir(sessionsDir, filePath)) continue;
      const info = await stat(filePath).catch(() => null);
      if (!info || !info.isFile() || info.size > MAX_SESSION_FILE_BYTES) continue;
      const raw = await readFile(filePath, 'utf8').catch(() => '');
      const parsed = parseDurableReceiptFile(raw, name, info.mtimeMs, stateRecords);
      if (!parsed) continue;
      if (!byId.has(parsed.summary.id)) {
        byId.set(parsed.summary.id, parsed);
      } else {
        const existing = byId.get(parsed.summary.id);
        if (!existing.summary.sessionId && parsed.summary.sessionId) {
          existing.summary.sessionId = parsed.summary.sessionId;
          existing.summary.resume = parsed.summary.resume;
        }
      }
    }
  }

  // Also surface active intent records from state.json that haven't flushed a live file yet.
  for (const [key, rec] of Object.entries(stateRecords)) {
    const invId = rec?.last?.id;
    if (!invId || byId.has(invId)) continue;
    if (rec.last.status !== 'intent' && rec.last.status !== 'blocked') continue;
    const { role, scope } = splitRoleScope(key);
    const harness = rec.configuration?.harness || '';
    const cwd = rec.cwd || '';
    const sessionId = rec['thread-id'] || rec.last['requested-thread'] || '';
    byId.set(invId, {
      summary: {
        id: invId,
        key,
        role,
        scope,
        attempt: rec.last.attempt ?? 0,
        harness,
        model: rec.configuration?.model || '',
        thinking: rec.configuration?.thinking || '',
        cwd,
        status: rec.last.status === 'intent' ? 'running' : 'blocked',
        running: rec.last.status === 'intent',
        exitCode: null,
        signal: '',
        durationMs: null,
        startedAt: rec.last['started-at'] || '',
        updatedAt: rec.last['started-at'] || '',
        sessionId,
        resume: buildResumeInfo(harness, sessionId, cwd),
        errorCode: '',
        errorSummary: rec.reason || '',
        usage: null,
        source: 'state',
        eventCount: 0,
      },
      detail: { blocks: [], stderr: '', text: '', truncated: false, rawTail: [] },
    });
  }

  const sorted = [...byId.values()].sort((a, b) => {
    if (a.summary.running !== b.summary.running) return a.summary.running ? -1 : 1;
    const ta = Date.parse(a.summary.updatedAt || a.summary.startedAt || '') || 0;
    const tb = Date.parse(b.summary.updatedAt || b.summary.startedAt || '') || 0;
    return tb - ta;
  });

  const selected = targetId
    ? sorted.find(item => item.summary.id === targetId || item.summary.key === targetId) ?? null
    : null;

  return {
    entries: sorted.map(item => item.summary),
    selected: selected ? { ...selected.summary, ...selected.detail } : null,
  };
}

export async function readTaskTraces(taskDir) {
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || taskDir.includes('\0')) {
    throw new Error('traces requires an absolute taskDir');
  }
  const resolvedTask = await realpath(taskDir);
  if (!(await stat(resolvedTask)).isDirectory()) {
    throw new Error('taskDir must be an existing directory');
  }
  const { entries } = await collectTraces(resolvedTask);
  return { protocol: 1, ok: true, taskDir: resolvedTask, traces: entries };
}

export async function readTaskTraceDetail(taskDir, traceId) {
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || taskDir.includes('\0')) {
    throw new Error('trace-detail requires an absolute taskDir');
  }
  if (typeof traceId !== 'string' || !traceId.trim() || traceId.includes('\0')) {
    throw new Error('trace-detail requires a traceId');
  }
  const resolvedTask = await realpath(taskDir);
  if (!(await stat(resolvedTask)).isDirectory()) {
    throw new Error('taskDir must be an existing directory');
  }
  const { selected } = await collectTraces(resolvedTask, traceId.trim());
  return { protocol: 1, ok: true, taskDir: resolvedTask, trace: selected };
}
