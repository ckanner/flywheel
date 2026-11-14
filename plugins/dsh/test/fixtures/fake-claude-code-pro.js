#!/usr/bin/env node
// Offline Claude CLI peer. The real public plugin drives this process.
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { join } from 'node:path';

const argv = process.argv.slice(2);
const flag = name => { const index = argv.indexOf(name); return index < 0 ? undefined : argv[index + 1]; };
const settingsPath = join(process.cwd(), 'claude-fixture.json');
const settings = existsSync(settingsPath) ? JSON.parse(readFileSync(settingsPath, 'utf8')) : {};
const scenario = settings.scenario || 'ok';
let input = '';
for await (const chunk of process.stdin) input += chunk;
const userContent = input.trim() ? JSON.parse(input.trim()).message.content : [];
const prompt = userContent.filter(block => block.type === 'text').map(block => block.text).join('\n');
const resume = flag('--resume');
const id = resume || flag('--session-id') || randomUUID();
const log = join(process.cwd(), 'claude-fixture.jsonl');
appendFileSync(log, JSON.stringify({ argv, prompt, userContent, pid: process.pid, scenario,
  credentialPresent: Boolean(process.env.ANTHROPIC_API_KEY), leakedDSH: Boolean(process.env.DSH_SECRET) }) + '\n');
const emit = event => process.stdout.write(JSON.stringify(event) + '\n');
if (argv[0] === 'ultrareview') {
  if (scenario === 'review-hang') await new Promise(resolve => setTimeout(resolve, 60000));
  process.stdout.write(JSON.stringify({ bugs: [{ title: 'offline finding' }] }, null, 2));
  process.exit(0);
}
const history = join(process.cwd(), 'claude-history-' + id + '.json');
const prior = resume && existsSync(history) ? JSON.parse(readFileSync(history, 'utf8')).prompt : '';
if (scenario === 'resume-missing' && resume) {
  process.stderr.write('No conversation found with session ID: ' + resume + '\n');
  process.exit(1);
}
emit({ type: 'system', subtype: 'init', session_id: id,
  model: scenario === 'wrong-model' ? 'unexpected-model' : flag('--model'), cwd: scenario === 'wrong-cwd' ? '/' : process.cwd(),
  permissionMode: scenario === 'wrong-permission' ? 'plan' : flag('--permission-mode'),
  claude_code_version: 'fixture-offline',
  tools: scenario === 'native-mismatch' ? ['Agent', 'Workflow'] : ['Read', 'Bash',
    ...((flag('--disallowedTools') || '').split(',').includes('Agent') ? [] : ['Agent']),
    ...(process.env.CLAUDE_CODE_DISABLE_WORKFLOWS === '0' ? ['Workflow'] : [])],
  agents: flag('--agents') ? Object.keys(JSON.parse(flag('--agents'))) : [] });
if (scenario === 'native-child') {
  emit({ type: 'assistant', session_id: id, message: { content: [{ type: 'tool_use', id: 'spawn-1', name: 'Agent', input: {} }] } });
  emit({ type: 'system', subtype: 'task_started', session_id: id, task_id: 'child', run_id: 'run-child', task_type: 'local_agent', tool_use_id: 'spawn-1' });
  for (let i = 0; i < 2; i++) emit({ type: 'system', subtype: 'task_progress', session_id: id, task_id: 'child', run_id: 'run-child', usage: { total_tokens: 10, tool_uses: 1, duration_ms: 20 } });
  emit({ type: 'system', subtype: 'task_notification', session_id: id, task_id: 'child', run_id: 'run-child', status: 'completed' });
}
if (['hang', 'stubborn', 'progress-hang'].includes(scenario)) {
  if (scenario === 'stubborn') process.on('SIGTERM', () => {});
  emit({ type: 'assistant', session_id: id, message: { content: [{ type: 'text', text: 'fixture running' }] } });
  if (scenario === 'progress-hang') {
    for (let i = 0; i < 20; i++) emit({ type: 'assistant', session_id: id,
      message: { content: [{ type: 'text', text: 'x'.repeat(65536) }] } });
  }
  setInterval(() => {}, 1000);
} else {
  const answer = prior ? 'prior:' + prior + '|echo:' + prompt : 'echo:' + prompt;
  if (!argv.includes('--no-session-persistence')) writeFileSync(history, JSON.stringify({ prompt }));
  emit({ type: 'assistant', session_id: id, message: { content: [{ type: 'text', text: answer }] } });
  if (scenario !== 'missing-result') {
    const result = { type: 'result', subtype: 'success', is_error: false, uuid: randomUUID(),
      session_id: scenario === 'wrong-session' ? randomUUID() : id, result: answer,
      usage: scenario === 'partial-usage' ? { input_tokens: 10, output_tokens: 5 } :
        { input_tokens: 10, output_tokens: 5, cache_read_input_tokens: 4, cache_creation_input_tokens: 3 },
      total_cost_usd: 0.12 };
    if (flag('--json-schema')) result.structured_output = { answer: 42 };
    if (scenario === 'unknown-usage') delete result.usage;
    emit(result);
    if (scenario === 'duplicate-result') emit(result);
    if (scenario === 'malformed-after-terminal') process.stdout.write('not-json\n');
  }
  process.exitCode = scenario === 'abnormal-exit' ? 2 : 0;
}
