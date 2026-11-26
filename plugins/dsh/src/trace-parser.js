import { readdir, readFile, realpath, stat } from 'node:fs/promises';
import { homedir } from 'node:os';
import { basename, isAbsolute, join, sep } from 'node:path';

const MAX_SESSION_FILE_BYTES = 8 * 1024 * 1024;
const MAX_BLOCKS = 240;
const MAX_BLOCK_CHARS = 262144;
const MAX_TOOL_CHARS = 16000;
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
  const limit =
    block.kind === 'assistant' || block.kind === 'thinking' || block.kind === 'user_prompt'
      ? MAX_BLOCK_CHARS
      : MAX_TOOL_CHARS;
  const prev = blocks[blocks.length - 1];
  // Coalesce consecutive thinking or assistant chunks verbatim into continuous prose.
  if (
    prev &&
    prev.kind === block.kind &&
    (block.kind === 'thinking' || block.kind === 'assistant') &&
    prev.title === block.title &&
    !block.separate
  ) {
    const sepStr = block.joinNewline && prev.text && !prev.text.endsWith('\n') && !text.startsWith('\n') ? '\n' : '';
    prev.text = truncateText(prev.text + sepStr + text, limit);
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
    text: truncateText(text, limit),
  });
}

function recordActivityStep(steps, label) {
  if (!label) return;
  const prev = steps[steps.length - 1];
  if (prev && prev.label === label) {
    prev.count += 1;
    return;
  }
  if (steps.length < 60) {
    steps.push({ label, count: 1 });
  }
}

/**
 * Incrementally extracts a human-readable Markdown string from structured
 * planning/review JSON output (e.g. `{"document": "# Title\n..."}`) even while
 * the JSON is still streaming and not yet closed.
 */
export function unwrapStreamingContractMarkdown(raw) {
  if (typeof raw !== 'string') return null;
  const trimmed = raw.trimStart();

  // Escape-free Multi-File Plan Protocol (`<<<FLYWHEEL_FILE path="...">>> ... <<<END_FLYWHEEL_FILE>>>`)
  if (trimmed.includes('<<<FLYWHEEL_FILE path="') || trimmed.includes('<<<FLYWHEEL_PLAN_META>>>')) {
    const firstTagIdx = trimmed.indexOf('<<<FLYWHEEL_');
    const pre = firstTagIdx > 0 ? trimmed.slice(0, firstTagIdx).trim() : '';
    const entries = [];
    let streaming = false;
    const fileStartTag = '<<<FLYWHEEL_FILE path="';
    const fileEndTag = '<<<END_FLYWHEEL_FILE>>>';
    let cursor = Math.max(0, firstTagIdx);
    while (cursor < trimmed.length) {
      const startIdx = trimmed.indexOf(fileStartTag, cursor);
      if (startIdx < 0) break;
      const pathStart = startIdx + fileStartTag.length;
      const pathEnd = trimmed.indexOf('">>>', pathStart);
      if (pathEnd < 0) {
        streaming = true;
        break;
      }
      const filePath = trimmed.slice(pathStart, pathEnd).trim();
      let contentStart = pathEnd + 4;
      if (trimmed[contentStart] === '\r') contentStart += 1;
      if (trimmed[contentStart] === '\n') contentStart += 1;
      const endIdx = trimmed.indexOf(fileEndTag, contentStart);
      if (endIdx < 0) {
        streaming = true;
        const partialContent = trimmed.slice(contentStart);
        if (filePath && partialContent.trim()) entries.push([filePath, partialContent]);
        break;
      }
      const content = trimmed.slice(contentStart, endIdx).replace(/\r?\n$/, '');
      if (filePath) entries.push([filePath, content]);
      cursor = endIdx + fileEndTag.length;
    }
    if (entries.length > 0) {
      const md = formatContractFilesMarkdown(entries, '', streaming);
      return { markdown: pre ? `${pre}\n\n${md}` : md, rawJson: raw };
    }
  }

  let preamble = '';
  let body = trimmed;
  if (body.startsWith('```')) {
    const nl = body.indexOf('\n');
    if (nl < 0) return null;
    body = body.slice(nl + 1).trimStart();
  }
  if (!body.startsWith('{')) {
    // Handle models that emit a brief natural-language preamble before the JSON contract.
    const embed = /\{\s*"(document|summary)"\s*:/.exec(body);
    if (!embed) return null;
    preamble = body.slice(0, embed.index).replace(/```(?:json)?\s*$/i, '').trim();
    body = body.slice(embed.index);
  }
  const withPreamble = md => (preamble ? `${preamble}\n\n${md}` : md);

  // Fast path: complete valid JSON object.
  try {
    const clean = body.replace(/\n?```\s*$/, '');
    const parsed = JSON.parse(clean);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      if (typeof parsed.document === 'string' && parsed.document.trim()) {
        return { markdown: withPreamble(parsed.document), rawJson: raw };
      }
      // Stage 0 clarification contract JSON -> clean readable Markdown
      if (typeof parsed['needs-clarification'] === 'boolean' && (typeof parsed.summary === 'string' || Array.isArray(parsed.questions))) {
        const lines = [];
        if (parsed.summary) lines.push(parsed.summary);
        if (Array.isArray(parsed['suggested-gates']) && parsed['suggested-gates'].length > 0) {
          lines.push('### 建议可量化质量门禁');
          for (const g of parsed['suggested-gates']) {
            if (typeof g === 'string' && g.trim()) lines.push(`- ${g.trim()}`);
          }
        }
        if (Array.isArray(parsed.questions) && parsed.questions.length > 0) {
          lines.push('### 待澄清问题');
          parsed.questions.forEach((q, idx) => {
            if (!q || typeof q !== 'object') return;
            lines.push(`#### ${idx + 1}. ${q.title || q.id || '问题'}`);
            if (q.rationale) lines.push(q.rationale);
            if (Array.isArray(q.options)) {
              for (const opt of q.options) {
                const isRec = q.recommended && opt === q.recommended;
                lines.push(`- ${isRec ? '**[推荐]** ' : ''}${opt}`);
              }
            }
          });
        }
        if (lines.length > 0) return { markdown: withPreamble(lines.join('\n\n')), rawJson: raw };
      }
      // Stage 1.5 UI/UX prototype contract JSON -> clean readable Markdown
      if (typeof parsed.html === 'string' && parsed.html.trim()) {
        const lines = [];
        if (parsed.title) lines.push(`### ${parsed.title}`);
        if (parsed.summary) lines.push(parsed.summary);
        if (parsed['design-tokens'] && typeof parsed['design-tokens'] === 'object') {
          lines.push('#### Design Tokens');
          for (const [k, v] of Object.entries(parsed['design-tokens'])) {
            lines.push(`- \`${k}\`: \`${v}\``);
          }
        }
        lines.push(`*已生成完整自包含交互原型 HTML（${parsed.html.length} 字符），可在主流程卡片中直接预览并审批。*`);
        return { markdown: withPreamble(lines.join('\n\n')), rawJson: raw };
      }
      if (typeof parsed.summary === 'string' && parsed.summary.trim() && (parsed.verdict || Array.isArray(parsed.findings))) {
        const lines = [];
        const metaParts = [];
        if (parsed.verdict) metaParts.push(`**审查结论**: \`${parsed.verdict}\``);
        if (parsed['scope-verdict']) metaParts.push(`**范围判定**: \`${parsed['scope-verdict']}\``);
        if (metaParts.length > 0) lines.push(metaParts.join(' · '));
        lines.push(parsed.summary);
        if (Array.isArray(parsed.findings) && parsed.findings.length > 0) {
          lines.push(`### 审查发现 (${parsed.findings.length} 项)`);
          for (const f of parsed.findings) {
            if (!f || typeof f !== 'object') continue;
            const idPart = f.id ? `\`${f.id}\` ` : '';
            const sev = f.severity ? `[${f.severity}] ` : '';
            const locPart = f.location ? `(\`${f.location}\`) ` : '';
            const head = f.title || f.target || f.category || f.id || '';
            const detail = f.summary || f.impact || f.evidence || '';
            const fix = f.recommendation || f.suggestion || f.fix || f['suggested-fix'] || '';
            let bullet = `- ${idPart}${sev}${locPart}**${head}**${detail ? `：${detail}` : ''}`;
            if (fix) bullet += `\n  - **建议修复**：${fix}`;
            lines.push(bullet);
          }
        }
        return { markdown: withPreamble(lines.join('\n\n')), rawJson: raw };
      }
      // Stage 3 packet code-review / judge verdict JSON (`{"verdict": "pass"|"return"|"verification-gap", "score": 0.94, "reasons": [...], "advisory": [...]}`)
      if (typeof parsed.verdict === 'string' && Array.isArray(parsed.reasons)) {
        const lines = [];
        const scorePart = typeof parsed.score === 'number' ? ` · **评分**: \`${parsed.score}\`` : '';
        lines.push(`**审查结论**: \`${parsed.verdict}\`${scorePart}`);
        if (parsed.reasons.length > 0) {
          lines.push(`### 审查意见 (${parsed.reasons.length} 项)`);
          for (const r of parsed.reasons) {
            if (typeof r === 'string' && r.trim()) lines.push(`- ${r.trim()}`);
          }
        }
        if (Array.isArray(parsed.advisory) && parsed.advisory.length > 0) {
          lines.push(`### 建议改进 (${parsed.advisory.length} 项)`);
          for (const adv of parsed.advisory) {
            if (typeof adv === 'string' && adv.trim()) lines.push(`- ${adv.trim()}`);
          }
        }
        return { markdown: withPreamble(lines.join('\n\n')), rawJson: raw };
      }
      // Stage 2 milestone plan contract JSON (`{"files": {"spec.md": "...", "task-packet-*.md": "..."}}`)
      if (parsed.files && typeof parsed.files === 'object' && !Array.isArray(parsed.files)) {
        const fileEntries = Object.entries(parsed.files).filter(([, v]) => typeof v === 'string' && v.trim());
        if (fileEntries.length > 0) {
          return {
            markdown: withPreamble(formatContractFilesMarkdown(fileEntries, parsed.summary, false)),
            rawJson: raw,
          };
        }
      }
    }
  } catch {
    // Still streaming incomplete JSON — fall through to incremental string unescaper.
  }

  // Incremental / fault-tolerant unwrapper for `{"files": {"spec.md": "...", ...}}`
  // so streaming or mildly malformed (e.g. unescaped inline quote) plan outputs still
  // render cleanly as YAML + Markdown instead of a raw 100KB JSON blob.
  const streamedFiles = extractStreamingContractFiles(body);
  if (streamedFiles && streamedFiles.entries.length > 0) {
    return {
      markdown: withPreamble(formatContractFilesMarkdown(streamedFiles.entries, streamedFiles.summary, true)),
      rawJson: raw,
    };
  }

  const keyMatch = /"(document|summary)"\s*:\s*"/.exec(body);
  if (!keyMatch) return null;
  let i = keyMatch.index + keyMatch[0].length;
  let out = '';
  while (i < body.length) {
    const ch = body[i];
    if (ch === '"') break;
    if (ch === '\\') {
      if (i + 1 >= body.length) break;
      const next = body[i + 1];
      if (next === 'n') { out += '\n'; i += 2; continue; }
      if (next === 'r') { out += '\r'; i += 2; continue; }
      if (next === 't') { out += '\t'; i += 2; continue; }
      if (next === '"' || next === '\\' || next === '/') { out += next; i += 2; continue; }
      if (next === 'u' && i + 5 < body.length) {
        const hex = body.slice(i + 2, i + 6);
        if (/^[0-9a-fA-F]{4}$/.test(hex)) {
          out += String.fromCharCode(Number.parseInt(hex, 16));
          i += 6;
          continue;
        }
      }
      if (next === 'u') break; // incomplete \uXXXX at tail of stream
      out += next;
      i += 2;
      continue;
    }
    out += ch;
    i += 1;
  }
  if (!out.trim()) return null;
  return { markdown: withPreamble(out), rawJson: raw };
}

function formatContractFilesMarkdown(fileEntries, summary, streaming = false) {
  const lines = [];
  if (typeof summary === 'string' && summary.trim()) {
    lines.push(summary.trim());
  }
  lines.push(
    streaming
      ? `### 实施计划拆解产物（已解析 ${fileEntries.length} 个文件 · 流式/容错视图）`
      : `### 实施计划拆解产物（共 ${fileEntries.length} 个文件）`,
  );
  lines.push(fileEntries.map(([name]) => `- \`${name}\``).join('\n'));
  for (const [name, content] of fileEntries) {
    let formatted = String(content).trim();
    if (/\.(?:mjs|cjs|js|ts|tsx)$/i.test(name)) {
      formatted = `\`\`\`javascript\n${formatted}\n\`\`\``;
    } else if (/\.sh$/i.test(name)) {
      formatted = `\`\`\`bash\n${formatted}\n\`\`\``;
    } else {
      const fm = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(formatted);
      if (fm) {
        formatted = `\`\`\`yaml\n${fm[1].trim()}\n\`\`\`\n\n${fm[2].trim()}`;
      } else if (/^---\r?\n/.test(formatted)) {
        formatted = `\`\`\`yaml\n${formatted.replace(/^---\r?\n/, '')}\n\`\`\``;
      }
    }
    lines.push(`---\n\n#### \`${name}\`\n\n${formatted}`);
  }
  return lines.join('\n\n');
}

function extractStreamingContractFiles(body) {
  const filesMatch = /"files"\s*:\s*\{/.exec(body);
  if (!filesMatch) return null;
  let pos = filesMatch.index + filesMatch[0].length;
  const entries = [];
  const nextEntryRe = /^\s*,\s*"([^"\r\n\\]{1,160}\.[A-Za-z0-9]{1,10})"\s*:\s*"/;
  const firstEntryRe = /^\s*"([^"\r\n\\]{1,160})"\s*:\s*"/;
  const endFilesRe = /^\s*\}\s*(?:$|\}|,\s*"[A-Za-z0-9_-]+"\s*:)/;

  const first = firstEntryRe.exec(body.slice(pos));
  if (!first) return null;
  let currentKey = first[1];
  const seenKeys = new Set([currentKey]);
  pos += first[0].length;

  while (pos <= body.length && entries.length < 64) {
    let out = '';
    let nextKey = null;
    let doneFiles = false;
    while (pos < body.length) {
      const ch = body[pos];
      if (ch === '"') {
        const rest = body.slice(pos + 1);
        const nextMatch = nextEntryRe.exec(rest);
        if (nextMatch && !seenKeys.has(nextMatch[1])) {
          nextKey = nextMatch[1];
          seenKeys.add(nextKey);
          pos = pos + 1 + nextMatch[0].length;
          break;
        }
        if (endFilesRe.test(rest)) {
          doneFiles = true;
          pos += 1;
          break;
        }
        // Unescaped literal quote inside malformed file content — preserve and continue scanning.
        out += '"';
        pos += 1;
        continue;
      }
      if (ch === '\\') {
        if (pos + 1 >= body.length) {
          pos = body.length;
          break;
        }
        const next = body[pos + 1];
        if (next === 'n') { out += '\n'; pos += 2; continue; }
        if (next === 'r') { out += '\r'; pos += 2; continue; }
        if (next === 't') { out += '\t'; pos += 2; continue; }
        if (next === '"' || next === '\\' || next === '/') { out += next; pos += 2; continue; }
        if (next === 'u' && pos + 5 < body.length) {
          const hex = body.slice(pos + 2, pos + 6);
          if (/^[0-9a-fA-F]{4}$/.test(hex)) {
            out += String.fromCharCode(Number.parseInt(hex, 16));
            pos += 6;
            continue;
          }
        }
        if (next === 'u') {
          pos = body.length;
          break;
        }
        out += next;
        pos += 2;
        continue;
      }
      out += ch;
      pos += 1;
    }
    if (out.trim()) {
      entries.push([currentKey, out]);
    }
    if (nextKey) {
      currentKey = nextKey;
      continue;
    }
    if (doneFiles || pos >= body.length) break;
  }
  return entries.length > 0 ? { entries, summary: '' } : null;
}

function finalizeParsedBlocks(blocks, activitySteps) {
  const result = [];
  const hasRichTrace = blocks.some(b => b.kind === 'thinking' || b.kind === 'tool_call' || b.kind === 'tool_result');
  if (activitySteps.length > 0 && !hasRichTrace) {
    const items = activitySteps.map(s => (s.count > 1 ? `${s.label} × ${s.count}` : s.label));
    result.push({
      kind: 'steps',
      title: 'activity',
      text: items.join(' · '),
      items,
    });
  }
  for (const b of blocks) {
    if (b.kind === 'assistant') {
      const unwrapped = unwrapStreamingContractMarkdown(b.text);
      if (unwrapped) {
        result.push({
          ...b,
          text: unwrapped.markdown,
          rawJson: unwrapped.rawJson,
        });
        continue;
      }
    }
    result.push(b);
  }
  return result;
}

function extractContentParts(content) {
  if (typeof content === 'string') return [{ type: 'text', text: content }];
  if (!Array.isArray(content)) return [];
  return content.filter(part => part && typeof part === 'object');
}

function unquoteJsonScalar(value) {
  if (typeof value !== 'string') return value;
  const t = value.trim();
  if ((t.startsWith('"') && t.endsWith('"')) || (t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
    try {
      return JSON.parse(t);
    } catch {
      return value;
    }
  }
  return value;
}

function truncateInline(text, limit = 76) {
  if (typeof text !== 'string') return '';
  const single = text.replace(/\s+/g, ' ').trim();
  if (single.length <= limit) return single;
  return single.slice(0, limit - 1) + '…';
}

function formatToolCallTitle(name, rawArgs) {
  const base = String(name || 'tool');
  let args = rawArgs;
  if (typeof args === 'string') {
    try {
      args = JSON.parse(args);
    } catch {
      // Parse Codex JS REPL `exec` tool snippets into clean human-readable labels.
      if (base === 'exec') {
        const cmds = [];
        for (const m of args.matchAll(/exec_command\s*\(\s*\{\s*cmd\s*:\s*"((?:\\.|[^"\\])+)"/g)) {
          cmds.push(m[1].replace(/\\"/g, '"').replace(/\\\\/g, '\\').replace(/\s+/g, ' ').trim());
        }
        if (cmds.length === 1) return `Shell · ${truncateInline(cmds[0], 78)}`;
        if (cmds.length > 1) return `Shell (×${cmds.length}) · ${truncateInline(cmds[0], 64)}`;
        // Handle `Promise.allSettled(["git rev-parse...", ...].map(c => tools.exec_command({cmd: c})))`
        if (/exec_command\b/.test(args)) {
          const arrMatch = /\[\s*"((?:\\.|[^"\\])+)"/.exec(args);
          if (arrMatch) {
            const firstCmd = arrMatch[1].replace(/\\"/g, '"').replace(/\\\\/g, '\\').replace(/\s+/g, ' ').trim();
            return `Shell (批量) · ${truncateInline(firstCmd, 66)}`;
          }
          return 'Shell · batch exec_command';
        }
        const webMatches = [...args.matchAll(/web__run\b[\s\S]*?q\s*:\s*"((?:\\.|[^"\\])+)"/g)];
        if (webMatches.length > 0) {
          const q = webMatches[0][1].replace(/\\"/g, '"').replace(/\\\\/g, '\\').trim();
          return `Web Search · ${truncateInline(q, 72)}`;
        }
        if (/apply_patch\b/.test(args)) return 'Apply Patch';
      }
      const compact = truncateInline(args, 72);
      return compact ? `${base} · ${compact}` : base;
    }
  }
  if (!args || typeof args !== 'object') return base;
  const pick = key => {
    const v = unquoteJsonScalar(args[key]);
    return typeof v === 'string' && v.trim() ? v.trim() : '';
  };
  const summary = pick('toolSummary') || pick('description');
  if (summary) return `${base} · ${truncateInline(summary, 72)}`;
  const filePath = pick('Path') || pick('file_path') || pick('path') || pick('AbsolutePath') || pick('TargetFile') || pick('SearchPath');
  if (filePath) return `${base} · ${basename(filePath) || filePath}`;
  const cmd = pick('CommandLine') || pick('command') || pick('cmd');
  if (cmd) return `${base} · ${truncateInline(cmd, 72)}`;
  const query = pick('Query') || pick('query') || pick('pattern') || pick('Pattern');
  if (query) return `${base} · ${truncateInline(query, 64)}`;
  return base;
}

function formatAntigravityToolArgs(rawArgs) {
  if (!rawArgs || typeof rawArgs !== 'object') return stringifyCompact(rawArgs, 2400);
  const cleaned = {};
  for (const [k, v] of Object.entries(rawArgs)) {
    cleaned[k] = unquoteJsonScalar(v);
  }
  return stringifyCompact(cleaned, 2400);
}

function cleanAntigravityToolOutput(raw) {
  if (typeof raw !== 'string') return stringifyCompact(raw, 2400);
  const lines = raw.split(/\r?\n/);
  let start = 0;
  while (start < lines.length && /^(?:Created At|Completed At):\s/.test(lines[start])) {
    start += 1;
  }
  return lines.slice(start).join('\n').trim() || raw.trim();
}

function isWorkerStatusProgress(text) {
  if (typeof text !== 'string') return false;
  const t = text.trim();
  if (!t) return false;
  if (/^Further display progress omitted/i.test(t)) return true;
  if (/^thread\s+\S+/i.test(t)) return true;
  if (/^commandExecution\s+(?:started|completed|failed)\b/i.test(t)) return true;
  if (/^(?:userMessage|reasoning|agentMessage|webSearch|fileChange|mcpToolCall|planUpdate)\s*$/i.test(t)) return true;
  return false;
}

function isStructuredContractText(text) {
  if (typeof text !== 'string') return false;
  const t = text.trimStart();
  return t.startsWith('{') || /^```(?:json)?\s*\n\s*\{/.test(t);
}

function cleanUserPromptText(raw) {
  if (typeof raw !== 'string') return '';
  return raw
    .replace(/^\s*<USER_REQUEST>\s*/i, '')
    .replace(/\s*<\/USER_REQUEST>\s*$/i, '')
    .trim();
}

function parseJsonLine(rawLine, ts, blocks, sessionRef, activitySteps = []) {
  let obj;
  try {
    obj = JSON.parse(rawLine);
  } catch {
    return;
  }
  if (!obj || typeof obj !== 'object') return;

  // Capture any session / thread ID emitted in stream or lifecycle frames.
  const discoveredSession =
    (typeof obj.sessionId === 'string' && obj.sessionId) ||
    (typeof obj.session_id === 'string' && obj.session_id) ||
    (typeof obj.thread_id === 'string' && obj.thread_id) ||
    (typeof obj.threadId === 'string' && obj.threadId) ||
    (typeof obj.lifecycle?.threadId === 'string' && obj.lifecycle.threadId) ||
    (typeof obj.receipt?.session?.id === 'string' && obj.receipt.session.id) ||
    (typeof obj.receipt?.sessionId === 'string' && obj.receipt.sessionId) ||
    (typeof obj.receipt?.threadId === 'string' && obj.receipt.threadId) ||
    (typeof obj.receipt?.conversationId === 'string' && obj.receipt.conversationId) ||
    (typeof obj.receipt?.outcome?.sessionId === 'string' && obj.receipt.outcome.sessionId) ||
    '';
  if (discoveredSession && !sessionRef.sessionId) {
    sessionRef.sessionId = discoveredSession;
  }

  // Capture context window occupancy from ACP usage_update or worker telemetry frames.
  const usageUpdate =
    (obj.type === 'usage_update' && obj) ||
    (obj.update?.sessionUpdate === 'usage_update' && obj.update) ||
    (obj.params?.update?.sessionUpdate === 'usage_update' && obj.params.update) ||
    (obj.receipt?.contextUsage && typeof obj.receipt.contextUsage === 'object' && obj.receipt.contextUsage) ||
    null;
  if (usageUpdate && typeof usageUpdate.used === 'number' && typeof usageUpdate.size === 'number' && usageUpdate.size > 0) {
    sessionRef.contextUsage = { used: usageUpdate.used, size: usageUpdate.size };
  }

  // 0. Antigravity CLI native transcript (`PLANNER_RESPONSE`, `GENERIC`, `ERROR_MESSAGE`)
  if (obj.type === 'PLANNER_RESPONSE') {
    const itemTs = obj.created_at || ts;
    if (typeof obj.thinking === 'string' && obj.thinking.trim()) {
      pushBlock(blocks, { kind: 'thinking', ts: itemTs, text: obj.thinking.trim(), separate: true });
    }
    if (Array.isArray(obj.tool_calls)) {
      for (const call of obj.tool_calls) {
        if (!call || typeof call !== 'object') continue;
        const name = call.name || 'tool';
        pushBlock(blocks, {
          kind: 'tool_call',
          ts: itemTs,
          title: formatToolCallTitle(name, call.args),
          text: formatAntigravityToolArgs(call.args),
        });
      }
    }
    if (typeof obj.content === 'string' && obj.content.trim()) {
      pushBlock(blocks, { kind: 'assistant', ts: itemTs, text: obj.content });
    }
    if (typeof obj.error === 'string' && obj.error.trim()) {
      pushBlock(blocks, { kind: 'progress', ts: itemTs, title: 'retry', text: obj.error.trim() });
    }
    return;
  }
  if (obj.type === 'GENERIC' && typeof obj.content === 'string' && obj.content.trim()) {
    const itemTs = obj.created_at || ts;
    pushBlock(blocks, {
      kind: 'tool_result',
      ts: itemTs,
      title: '输出',
      text: cleanAntigravityToolOutput(obj.content),
    });
    return;
  }
  if (obj.type === 'ERROR_MESSAGE' && typeof obj.error === 'string' && obj.error.trim()) {
    pushBlock(blocks, { kind: 'progress', ts: obj.created_at || ts, title: 'retry', text: obj.error.trim() });
    return;
  }
  if (obj.type === 'USER_INPUT') {
    const promptText = cleanUserPromptText(typeof obj.content === 'string' ? obj.content : '');
    if (promptText) {
      pushBlock(blocks, { kind: 'user_prompt', ts: obj.created_at || ts, text: promptText, separate: true });
    }
    return;
  }

  // 1. Kimi stream-json (`role: "user" | "assistant" | "tool"`)
  if (obj.role === 'user') {
    const content = typeof obj.content === 'string'
      ? obj.content
      : Array.isArray(obj.content)
        ? obj.content.filter(p => p?.type === 'text' && typeof p.text === 'string').map(p => p.text).join('\n')
        : '';
    const promptText = cleanUserPromptText(content);
    if (promptText) {
      pushBlock(blocks, { kind: 'user_prompt', ts, text: promptText, separate: true });
    }
    return;
  }
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
        pushBlock(blocks, { kind: 'tool_call', ts, title: formatToolCallTitle(String(name), args), text: stringifyCompact(args, 2400) });
      }
    }
    return;
  }

  if (obj.role === 'tool') {
    const content = typeof obj.content === 'string' ? obj.content : stringifyCompact(obj.content, 2400);
    pushBlock(blocks, { kind: 'tool_result', ts, title: obj.name || obj.tool_call_id || 'tool', text: content });
    return;
  }

  // 2. Claude SDK stream-json & native `~/.claude/projects/*.jsonl` (`type: "assistant" | "user"`)
  if (obj.type === 'assistant' && Array.isArray(obj.message?.content)) {
    const itemTs = obj.timestamp || ts;
    const hasToolUse = obj.message.content.some(p => p && typeof p === 'object' && p.type === 'tool_use');
    for (const part of obj.message.content) {
      if (!part || typeof part !== 'object') continue;
      if (part.type === 'thinking' && (part.thinking || part.text)) {
        pushBlock(blocks, { kind: 'thinking', ts: itemTs, text: String(part.thinking ?? part.text), separate: true });
      } else if (part.type === 'redacted_thinking' || (part.type === 'thinking' && part.signature)) {
        pushBlock(blocks, {
          kind: 'thinking',
          ts: itemTs,
          title: 'encrypted',
          text: '深度思考已由 Provider 加密（未返回明文）',
          separate: true,
        });
      } else if (part.type === 'text' && part.text) {
        const txt = String(part.text).trim();
        // When Claude's internal CoT is signature-only encrypted, brief commentary
        // emitted right before a tool_use (or short non-contract preamble) acts as
        // visible thought narration; render it inside the ThoughtChain.
        if (txt && (hasToolUse || (txt.length <= 240 && !isStructuredContractText(txt) && !txt.includes('\n\n#')))) {
          pushBlock(blocks, { kind: 'thinking', ts: itemTs, text: txt, separate: true });
        } else if (part.text) {
          pushBlock(blocks, { kind: 'assistant', ts: itemTs, text: String(part.text) });
        }
      } else if (part.type === 'tool_use') {
        pushBlock(blocks, { kind: 'tool_call', ts: itemTs, title: formatToolCallTitle(part.name || 'tool', part.input), text: stringifyCompact(part.input, 2400) });
      }
    }
    return;
  }

  if (obj.type === 'user') {
    const content = obj.message?.content;
    if (typeof content === 'string') {
      const promptText = cleanUserPromptText(content);
      if (promptText) {
        pushBlock(blocks, { kind: 'user_prompt', ts: obj.timestamp || ts, text: promptText, separate: true });
      }
      return;
    }
    if (Array.isArray(content)) {
      const textParts = [];
      for (const part of content) {
        if (part?.type === 'tool_result') {
          pushBlock(blocks, {
            kind: part.is_error ? 'error' : 'tool_result',
            ts: obj.timestamp || ts,
            title: part.tool_use_id || 'tool_result',
            text: stringifyCompact(part.content, 2400),
          });
        } else if (part?.type === 'text' && typeof part.text === 'string') {
          textParts.push(part.text);
        }
      }
      const promptText = cleanUserPromptText(textParts.join('\n'));
      if (promptText) {
        pushBlock(blocks, { kind: 'user_prompt', ts: obj.timestamp || ts, text: promptText, separate: true });
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

  // 4. Codex app-server / CLI items & Codex native rollout JSONL (`response_item`, `event_msg`)
  if (obj.type === 'event_msg' && obj.payload?.type === 'item_completed' && obj.payload?.item?.type === 'UserMessage') {
    const parts = extractContentParts(obj.payload.item.content);
    const text = parts
      .map(part => (typeof part.text === 'string' ? part.text : ''))
      .filter(Boolean)
      .join('\n');
    const promptText = cleanUserPromptText(text);
    if (promptText) {
      pushBlock(blocks, { kind: 'user_prompt', ts: obj.timestamp || ts, text: promptText, separate: true });
    }
    return;
  }
  if (obj.type === 'response_item' && obj.payload && typeof obj.payload === 'object') {
    const p = obj.payload;
    const itemTs = obj.timestamp || ts;
    if (p.type === 'reasoning') {
      const summaryParts = Array.isArray(p.summary)
        ? p.summary.map(s => (typeof s === 'string' ? s : s?.text || '')).filter(Boolean)
        : [];
      const text = summaryParts.join('\n') || (typeof p.text === 'string' ? p.text : '');
      if (text.trim()) {
        pushBlock(blocks, { kind: 'thinking', ts: itemTs, text, separate: true });
      } else if (p.encrypted_content) {
        pushBlock(blocks, {
          kind: 'thinking',
          ts: itemTs,
          title: 'encrypted',
          text: '深度思考已由 Provider 加密（未返回明文）',
          separate: true,
        });
      }
    } else if (p.type === 'message' && p.role === 'assistant') {
      const parts = extractContentParts(p.content);
      const text = parts
        .filter(part => part.type === 'output_text' || part.type === 'text')
        .map(part => (typeof part.text === 'string' ? part.text : ''))
        .join('\n')
        .trim();
      if (text) {
        if (p.phase === 'commentary') {
          pushBlock(blocks, { kind: 'thinking', ts: itemTs, text, separate: true });
        } else if (p.phase === 'final_answer' || isStructuredContractText(text)) {
          pushBlock(blocks, { kind: 'assistant', ts: itemTs, text, separate: true });
        }
      }
    } else if (p.type === 'custom_tool_call' || p.type === 'function_call') {
      const arg = p.input !== undefined ? p.input : p.arguments;
      pushBlock(blocks, { kind: 'tool_call', ts: itemTs, title: formatToolCallTitle(p.name || 'tool', arg), text: stringifyCompact(arg, 2400) });
    } else if (p.type === 'custom_tool_call_output' || p.type === 'function_call_output') {
      let outText = '';
      if (Array.isArray(p.output)) {
        outText = p.output.map(item => (typeof item === 'string' ? item : item?.text || stringifyCompact(item, 1200))).join('\n');
      } else {
        outText = stringifyCompact(p.output, 2400);
      }
      pushBlock(blocks, { kind: 'tool_result', ts: itemTs, title: String(p.name || p.call_id || 'tool_result'), text: outText });
    }
    return;
  }
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
  if (obj.type === 'progress' && typeof obj.text === 'string' && obj.text.length > 0) {
    if (isWorkerStatusProgress(obj.text)) {
      recordActivityStep(activitySteps, truncateText(obj.text.trim().replace(/\s+/g, ' '), 120));
    } else if (sessionRef.harness === 'codex-pro' && !isStructuredContractText(obj.text)) {
      // Codex worker emits brief natural-language commentary via `progress` frames
      // before the final structured answer arrives in `result`/`end.text`.
      pushBlock(blocks, { kind: 'thinking', ts, text: obj.text.trim(), separate: true });
    } else {
      // Workers stream model output deltas as `progress` frames; coalesce verbatim
      // into continuous assistant markdown rather than emitting per-chunk bullets.
      pushBlock(blocks, { kind: 'assistant', ts, text: obj.text });
    }
    return;
  }
  if (obj.type === 'activity' && obj.activity) {
    const act = obj.activity;
    const parts = [act.phase, act.summary, act.detail, act.tool, act.target].filter(v => typeof v === 'string' && v.trim());
    if (parts.length > 0) {
      recordActivityStep(activitySteps, `${act.source ? `[${act.source}] ` : ''}${parts.join(' · ')}`);
      return;
    }
    if (Array.isArray(act.eventTypes) && act.eventTypes.length > 0) {
      const EVENT_LABELS = {
        agent_thought_chunk: '思考推理中',
        agent_message_chunk: '生成回复中',
        tool_call: '调用工具中',
        tool_call_update: '工具执行更新',
        session_info_update: '会话已就绪',
        'item.started': '开始执行步骤',
        'item.completed': '完成执行步骤',
        'turn.started': '回合已启动',
        'stream-delta': '流式输出中',
        'tool-result': '工具返回结果',
        'task-progress': '子任务推进中',
      };
      const meaningful = act.eventTypes.filter(
        t =>
          typeof t === 'string' &&
          t !== 'usage_update' &&
          t !== 'available_commands_update' &&
          t !== 'config_option_update' &&
          t !== 'current_mode_update',
      );
      if (meaningful.length > 0) {
        const label = meaningful.map(t => EVENT_LABELS[t] || t).join(' · ');
        const scopeSuffix = act.scope && act.scope !== 'root' ? ` (${act.scope})` : '';
        recordActivityStep(activitySteps, `${act.source ? `[${act.source}] ` : ''}${label}${scopeSuffix}`);
      }
    }
    return;
  }
  if (obj.type === 'event' && obj.event && typeof obj.event === 'object') {
    const ev = obj.event;
    const evType = String(ev.type || '');
    const data = ev.data && typeof ev.data === 'object' ? ev.data : ev;
    if (evType === 'user/message') {
      const parts = extractContentParts(data.message?.content ?? data.content ?? data.text);
      const text = parts.map(p => (typeof p.text === 'string' ? p.text : '')).filter(Boolean).join('\n');
      const promptText = cleanUserPromptText(text);
      if (promptText) {
        pushBlock(blocks, { kind: 'user_prompt', ts, text: promptText, separate: true });
      }
      return;
    }
    if (evType === 'agent/inbox/spliced' || evType === 'request/header') {
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
    const failMsg = obj.error?.message || obj.receipt?.failure?.message || obj.receipt?.failure?.detail;
    if (failMsg) {
      pushBlock(blocks, {
        kind: 'error',
        ts,
        title: obj.error?.code || obj.receipt?.failure?.category || 'failed',
        text: String(failMsg),
      });
    }
    const finalOut =
      (typeof obj.text === 'string' && obj.text) ||
      (Array.isArray(obj.receipt?.output) && obj.receipt.output.map(o => (typeof o === 'string' ? o : o?.text || '')).join('\n')) ||
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
    if (!b || !b.text || b.kind === 'user_prompt') continue;
    const prefix = b.kind === 'tool_call' && b.title ? `[${b.title}] ` : '';
    const line = String(b.text)
      .replace(/^…\s*\[truncated\]\s*/i, '')
      .replace(/^#+\s*/gm, '')
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/`([^`]+)`/g, '$1')
      .trim()
      .replace(/\s+/g, ' ');
    if (line) return truncateText(prefix + line, 220);
  }
  return '';
}

function appendFallbackAssistantText(rawBlocks, ts, endText) {
  const text = typeof endText === 'string' ? endText : '';
  if (!text.trim()) return;
  const hasAssistant = rawBlocks.some(b => b.kind === 'assistant' && b.text.trim().length > 0);
  const isTruncatedTail = text.trimStart().startsWith('… [truncated]');
  // Never append Go's 16K tail-truncated end.text as a second raw block when
  // streaming progress already delivered the full assistant response.
  if (hasAssistant) return;
  if (isTruncatedTail) {
    const cleaned = text.replace(/^…\s*\[truncated\]\r?\n?/, '');
    if (cleaned.trim()) {
      pushBlock(rawBlocks, { kind: 'assistant', ts, text: cleaned, separate: true });
    }
    return;
  }
  pushBlock(rawBlocks, { kind: 'assistant', ts, text, separate: true });
}

function resolveSessionHomeCwd(harness, cwd, workspaceCwd) {
  const h = typeof harness === 'string' ? harness.trim().toLowerCase() : '';
  if (workspaceCwd && h !== 'dsh') return workspaceCwd;
  return cwd || workspaceCwd || '';
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

  const sessionRef = { sessionId: end?.sessionId || start.requestedThread || '', harness: end?.harness || start.harness || '' };
  const rawBlocks = [];
  const activitySteps = [];
  for (const ev of events) {
    parseJsonLine(ev.line, ev.ts, rawBlocks, sessionRef, activitySteps);
  }
  appendFallbackAssistantText(rawBlocks, end?.ts || '', end?.text);
  if (end?.stderr && String(end.stderr).trim()) {
    pushBlock(rawBlocks, { kind: 'error', ts: end.ts || '', title: 'stderr', text: String(end.stderr) });
  }
  if (end?.errorMessage) {
    pushBlock(rawBlocks, { kind: 'error', ts: end.ts || '', title: end.errorCode || 'error', text: String(end.errorMessage) });
  }
  const blocks = finalizeParsedBlocks(rawBlocks, activitySteps);

  const running = !end;
  const harness = end?.harness || start.harness || '';
  const model = end?.model || start.model || '';
  const cwd = start.cwd || start.workspaceCwd || '';
  const workspaceCwd = start.workspaceCwd || '';
  const sessionCwd = resolveSessionHomeCwd(harness, cwd, workspaceCwd);
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
      workspaceCwd,
      status,
      running,
      exitCode: end?.exitCode ?? null,
      signal: end?.signal || '',
      durationMs: end?.durationMs ?? null,
      startedAt: start.ts || '',
      updatedAt: end?.ts || events.at(-1)?.ts || new Date(fileMtimeMs).toISOString(),
      sessionId: sessionRef.sessionId || '',
      sessionTitle: typeof start.sessionTitle === 'string' ? start.sessionTitle : '',
      resume: buildResumeInfo(harness, sessionRef.sessionId, sessionCwd),
      errorCode: end?.errorCode || '',
      errorSummary: end?.errorMessage || (status === 'failed' && end?.stderr ? truncateText(String(end.stderr), 220) : ''),
      usage: end?.usage || null,
      contextUsage: sessionRef.contextUsage || null,
      promptBytes: typeof start.promptBytes === 'number' ? start.promptBytes : null,
      source: 'live',
      eventCount: events.length,
      byteSize: Buffer.byteLength(rawText, 'utf8'),
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
  const rawBlocks = [];
  const activitySteps = [];
  const rawStdout = typeof result.stdout === 'string' ? result.stdout : '';
  const stdoutLines = rawStdout.split('\n').filter(Boolean);
  for (const line of stdoutLines) {
    parseJsonLine(line, '', rawBlocks, sessionRef, activitySteps);
  }
  appendFallbackAssistantText(rawBlocks, '', result.text);
  if (result.stderr && String(result.stderr).trim()) {
    pushBlock(rawBlocks, { kind: 'error', ts: '', title: 'stderr', text: String(result.stderr) });
  }
  if (result.errorMessage) {
    pushBlock(rawBlocks, { kind: 'error', ts: '', title: result.errorCode || 'error', text: String(result.errorMessage) });
  }
  const blocks = finalizeParsedBlocks(rawBlocks, activitySteps);

  const harness = result.harness || stateRec?.configuration?.harness || '';
  const model = result.model || stateRec?.configuration?.model || '';
  const thinking = stateRec?.configuration?.thinking || '';
  const cwd = receipt.cwd || stateRec?.cwd || '';
  const workspaceCwd = stateRec?.['workspace-cwd'] || '';
  const sessionCwd = resolveSessionHomeCwd(harness, cwd, workspaceCwd);
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
      workspaceCwd,
      status,
      running: false,
      exitCode: result.exitCode ?? null,
      signal: result.signal || '',
      durationMs: result.durationMs ?? null,
      startedAt: stateRec?.last?.['started-at'] || '',
      updatedAt: new Date(fileMtimeMs).toISOString(),
      sessionId: sessionRef.sessionId || '',
      resume: buildResumeInfo(harness, sessionRef.sessionId, sessionCwd),
      errorCode: result.errorCode || '',
      errorSummary: result.errorMessage || (status === 'failed' && result.stderr ? truncateText(String(result.stderr), 220) : ''),
      usage: result.usage || null,
      contextUsage: sessionRef.contextUsage || null,
      promptBytes: null,
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
    const workspaceCwd = rec['workspace-cwd'] || '';
    const sessionCwd = resolveSessionHomeCwd(harness, cwd, workspaceCwd);
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
        workspaceCwd,
        status: rec.last.status === 'intent' ? 'running' : 'blocked',
        running: rec.last.status === 'intent',
        exitCode: null,
        signal: '',
        durationMs: null,
        startedAt: rec.last['started-at'] || '',
        updatedAt: rec.last['started-at'] || '',
        sessionId,
        resume: buildResumeInfo(harness, sessionId, sessionCwd),
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

async function readFileHead(filePath, maxBytes = 8192) {
  try {
    const raw = await readFile(filePath, 'utf8');
    return raw.length > maxBytes ? raw.slice(0, maxBytes) : raw;
  } catch {
    return '';
  }
}

async function locateLiveAntigravityTranscript(home, taskDir, cwd, startedAt) {
  const startedMs = Date.parse(startedAt || '') || 0;
  const minMtimeMs = startedMs > 0 ? startedMs - 20_000 : Date.now() - 30 * 60_000;
  const maxMtimeMs = startedMs > 0 ? startedMs + 4 * 3600_000 : Date.now() + 60_000;
  const needles = [taskDir, cwd ? `"${cwd}"` : ''].filter(v => typeof v === 'string' && v.trim());
  for (const brainDir of [join(home, '.gemini', 'antigravity-cli', 'brain'), join(home, '.gemini', 'antigravity', 'brain')]) {
    const entries = await readdir(brainDir).catch(() => []);
    const recent = [];
    for (const name of entries) {
      if (!/^[0-9a-f-]{8,}$/i.test(name)) continue;
      const candidate = join(brainDir, name, '.system_generated', 'logs', 'transcript.jsonl');
      const info = await stat(candidate).catch(() => null);
      if (!info?.isFile() || info.size > MAX_SESSION_FILE_BYTES || info.mtimeMs < minMtimeMs || info.mtimeMs > maxMtimeMs) continue;
      recent.push({ candidate, mtimeMs: info.mtimeMs, sessionId: name });
    }
    recent.sort((a, b) => b.mtimeMs - a.mtimeMs);
    for (const item of recent.slice(0, 8)) {
      const head = await readFileHead(item.candidate, 6144);
      if (!head) continue;
      if (needles.length === 0 || needles.some(n => head.includes(n))) {
        return { path: item.candidate, sessionId: item.sessionId };
      }
    }
  }
  return null;
}

async function locateLiveClaudeTranscript(home, taskDir, cwd, startedAt) {
  const projectsRoot = join(home, '.claude', 'projects');
  if (typeof cwd !== 'string' || !isAbsolute(cwd)) return null;
  const encoded = cwd.replace(/[^A-Za-z0-9-]/g, '-');
  const projDir = join(projectsRoot, encoded);
  const entries = await readdir(projDir).catch(() => []);
  const startedMs = Date.parse(startedAt || '') || 0;
  const minMtimeMs = startedMs > 0 ? startedMs - 20_000 : Date.now() - 30 * 60_000;
  const recent = [];
  for (const name of entries) {
    if (!name.endsWith('.jsonl')) continue;
    const candidate = join(projDir, name);
    const info = await stat(candidate).catch(() => null);
    if (!info?.isFile() || info.size > MAX_SESSION_FILE_BYTES || info.mtimeMs < minMtimeMs) continue;
    recent.push({ candidate, mtimeMs: info.mtimeMs, sessionId: name.slice(0, -6) });
  }
  recent.sort((a, b) => b.mtimeMs - a.mtimeMs);
  for (const item of recent.slice(0, 6)) {
    const head = await readFileHead(item.candidate, 8192);
    if (!head) continue;
    if (!taskDir || head.includes(taskDir) || head.includes(cwd)) {
      return { path: item.candidate, sessionId: item.sessionId };
    }
  }
  return null;
}

async function locateNativeTranscriptPath(selected, taskDir) {
  const id = typeof selected?.sessionId === 'string' ? selected.sessionId.trim() : '';
  const hasValidId = Boolean(id && SAFE_SESSION_ID.test(id) && !id.includes('/') && !id.includes('..'));
  const h = typeof selected?.harness === 'string' ? selected.harness.trim().toLowerCase() : '';
  const cwd = resolveSessionHomeCwd(h, selected?.cwd || '', selected?.workspaceCwd || '');
  const home = homedir();

  if (h === 'claude-code-pro') {
    if (hasValidId) {
      const projectsRoot = join(home, '.claude', 'projects');
      const candidates = [];
      if (typeof cwd === 'string' && isAbsolute(cwd)) {
        const encoded = cwd.replace(/[^A-Za-z0-9-]/g, '-');
        candidates.push(join(projectsRoot, encoded, `${id}.jsonl`));
      }
      for (const candidate of candidates) {
        const info = await stat(candidate).catch(() => null);
        if (info?.isFile() && info.size <= MAX_SESSION_FILE_BYTES) return { path: candidate, sessionId: id };
      }
      const dirs = await readdir(projectsRoot).catch(() => []);
      for (const dir of dirs.slice(0, 64)) {
        const candidate = join(projectsRoot, dir, `${id}.jsonl`);
        const info = await stat(candidate).catch(() => null);
        if (info?.isFile() && info.size <= MAX_SESSION_FILE_BYTES) return { path: candidate, sessionId: id };
      }
    }
    if (selected?.running) {
      return locateLiveClaudeTranscript(home, taskDir, cwd, selected.startedAt);
    }
    return null;
  }

  if (h === 'codex-pro' || h === 'codex') {
    if (!hasValidId) return null;
    const sessionsRoot = join(home, '.codex', 'sessions');
    const suffix = `${id}.jsonl`;
    const years = (await readdir(sessionsRoot).catch(() => [])).sort().reverse().slice(0, 2);
    for (const year of years) {
      const yearDir = join(sessionsRoot, year);
      const months = (await readdir(yearDir).catch(() => [])).sort().reverse().slice(0, 3);
      for (const month of months) {
        const monthDir = join(yearDir, month);
        const days = (await readdir(monthDir).catch(() => [])).sort().reverse().slice(0, 14);
        for (const day of days) {
          const dayDir = join(monthDir, day);
          const files = await readdir(dayDir).catch(() => []);
          const hit = files.find(f => f.endsWith(suffix));
          if (hit) {
            const candidate = join(dayDir, hit);
            const info = await stat(candidate).catch(() => null);
            if (info?.isFile() && info.size <= MAX_SESSION_FILE_BYTES) return { path: candidate, sessionId: id };
          }
        }
      }
    }
    return null;
  }

  if (h === 'antigravity') {
    if (hasValidId) {
      for (const root of [join(home, '.gemini', 'antigravity-cli', 'brain'), join(home, '.gemini', 'antigravity', 'brain')]) {
        const candidate = join(root, id, '.system_generated', 'logs', 'transcript.jsonl');
        const info = await stat(candidate).catch(() => null);
        if (info?.isFile() && info.size <= MAX_SESSION_FILE_BYTES) return { path: candidate, sessionId: id };
      }
    }
    if (selected?.running) {
      return locateLiveAntigravityTranscript(home, taskDir, cwd, selected.startedAt);
    }
  }

  return null;
}

async function enrichDetailWithNativeTranscript(selected, taskDir) {
  if (!selected) return selected;
  const existingBlocks = Array.isArray(selected.blocks) ? selected.blocks : [];
  const alreadyHasTools = existingBlocks.some(b => b.kind === 'tool_call' || b.kind === 'tool_result');
  if (alreadyHasTools) return selected;
  const located = await locateNativeTranscriptPath(selected, taskDir);
  if (!located?.path) return selected;
  const raw = await readFile(located.path, 'utf8').catch(() => '');
  if (!raw) return selected;
  const nativeBlocks = [];
  const nativeSteps = [];
  const sessionRef = { sessionId: selected.sessionId || located.sessionId || '', harness: selected.harness || '' };
  for (const line of raw.split('\n')) {
    if (!line.trim()) continue;
    parseJsonLine(line, '', nativeBlocks, sessionRef, nativeSteps);
  }
  // When a persistent native session (e.g. Codex resume) spans multiple Flywheel
  // invocations, scope blocks to this invocation's time window when timestamps match.
  const startedMs = Date.parse(selected.startedAt || '') || 0;
  const endedMs = !selected.running ? (Date.parse(selected.updatedAt || '') || 0) : 0;
  let scopedBlocks = nativeBlocks;
  if (startedMs > 0) {
    const inWindow = nativeBlocks.filter(b => {
      const t = Date.parse(b.ts || '') || 0;
      if (!t) return false;
      if (t < startedMs - 5_000) return false;
      if (endedMs > 0 && t > endedMs + 15_000) return false;
      return true;
    });
    if (inWindow.length > 0) scopedBlocks = inWindow;
  }
  const richChain = scopedBlocks.filter(
    b => b.kind === 'user_prompt' || b.kind === 'thinking' || b.kind === 'tool_call' || b.kind === 'tool_result' || b.kind === 'progress',
  );
  const nativeAssistant = scopedBlocks.filter(b => b.kind === 'assistant');
  if (richChain.length === 0 && nativeAssistant.length === 0) return selected;
  // Drop live commentary thinking blocks when native transcript provides the exact ordered chain.
  const hasNativeThinkingOrTools = richChain.some(b => b.kind === 'thinking' || b.kind === 'tool_call' || b.kind === 'tool_result');
  const preservedOutputs = existingBlocks.filter(b => b.kind !== 'steps' && !(hasNativeThinkingOrTools && b.kind === 'thinking'));
  const hasPreservedAssistant = preservedOutputs.some(b => b.kind === 'assistant');
  const merged = hasPreservedAssistant
    ? [...richChain, ...preservedOutputs]
    : [...richChain, ...finalizeParsedBlocks(nativeAssistant, []), ...preservedOutputs];
  const resolvedSessionId = selected.sessionId || located.sessionId || '';
  // Native sessions for Codex/Claude/Antigravity/Kimi live under workspaceCwd (main repo root);
  // mirror Go's sessionNativeCwd so CLI resume always points to the stable repo root.
  const resumeCwd = resolveSessionHomeCwd(selected.harness, selected.cwd || '', selected.workspaceCwd || '');
  return {
    ...selected,
    sessionId: resolvedSessionId,
    resume: buildResumeInfo(selected.harness, resolvedSessionId, resumeCwd) || selected.resume,
    blocks: merged,
  };
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
  const enriched = await enrichDetailWithNativeTranscript(selected, resolvedTask);
  return { protocol: 1, ok: true, taskDir: resolvedTask, trace: enriched };
}
