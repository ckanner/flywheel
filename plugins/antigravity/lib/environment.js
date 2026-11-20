import { readFileSync, statSync } from 'node:fs'
import { join, isAbsolute } from 'node:path'
import { homedir } from 'node:os'
import { object, hash, MachineInputError } from './input.js'

const sensitive = /(?:TOKEN|SECRET|PASSWORD|API_KEY|AUTH_TOKEN|CREDENTIAL)/i
const routing = /^(?:AGY_|GEMINI_|GOOGLE_|ANTHROPIC_|OPENAI_|CLAUDE_|CODEX_)/
export function runtimeEnvironment(runtime = {}) {
  if (runtime.env !== undefined && (!object(runtime.env) || Object.values(runtime.env).some(v => v !== undefined && typeof v !== 'string'))) throw new MachineInputError('runtime.env must contain strings or undefined')
  const source = runtime.env ?? process.env
  if (/^(?:true|1)$/i.test(source.AGY_ADC_AUTH ?? '')) throw new MachineInputError('ADC authentication conflicts with subscription-only execution')
  const env = {}
  for (const [key,value] of Object.entries(source)) {
    if (typeof value === 'string' && !sensitive.test(key) && !routing.test(key) && !['NODE_OPTIONS','NODE_PATH'].includes(key)) env[key] = value
  }
  if (!env.HOME) env.HOME = homedir()
  if (!isAbsolute(env.HOME)) throw new MachineInputError('native HOME must be an absolute path')
  return env
}
function readJSON(path) {
  try {
    if (statSync(path).size > 2*1024*1024) throw new MachineInputError('native configuration is too large')
    const value = JSON.parse(readFileSync(path,'utf8'))
    if (!object(value)) throw new MachineInputError('native configuration must be an object')
    return value
  } catch (error) {
    if (error.code === 'ENOENT') return {}
    throw new MachineInputError('cannot inspect native configuration: ' + error.message)
  }
}
/** Reads configuration only. Credentials and account token profiles are never opened. */
export function inspectSubscription(env, request) {
  const settings = readJSON(join(env.HOME,'.gemini','antigravity-cli','settings.json'))
  const shared = readJSON(join(env.HOME,'.gemini','config','config.json'))
  if (settings.modelProvider) throw new MachineInputError('native modelProvider must use account authentication')
  if (settings.useG1Credits !== undefined && settings.useG1Credits !== false) throw new MachineInputError('native useG1Credits must be false before subscription-only execution')
  if (shared.userSettings?.enableAdc === true || settings.gcp && Object.keys(settings.gcp).length || settings.customModelsConfig && Object.keys(settings.customModelsConfig).length) throw new MachineInputError('native ADC, GCP or custom model configuration conflicts with subscription-only execution')
  if (request) {
    if (!request.sandbox && settings.enableTerminalSandbox === true) throw new MachineInputError('native terminal sandbox is enabled; the requested false value cannot override it safely')
    if (request.permissionMode === 'request-review' && (settings.toolPermission ?? 'request-review') !== 'request-review') throw new MachineInputError('native toolPermission does not match request-review; configure it in the official CLI')
  }
  const facts = { modelProvider:settings.modelProvider ?? '',useG1Credits:settings.useG1Credits ?? false,gcp:settings.gcp ?? null,customModelsConfig:settings.customModelsConfig ?? null,
    enableAdc:shared.userSettings?.enableAdc ?? false,toolPermission:settings.toolPermission ?? 'request-review',enableTerminalSandbox:settings.enableTerminalSandbox ?? false,
    permissions:settings.permissions ?? null,allowNonWorkspaceAccess:settings.allowNonWorkspaceAccess ?? false }
  return {fingerprint:hash(facts),status:'verified-local-controls',authentication:'native-account-unverified',creditsFallback:false,extraBilling:'unknown'}
}
export function redact(value) {
  return String(value).replace(/(Bearer\s+)[A-Za-z0-9_.~+/=-]+/gi,'$1[redacted]').replace(/((?:api[_-]?key|token|password|secret)\s*[=:]\s*)[^\s,;]+/gi,'$1[redacted]')
}
