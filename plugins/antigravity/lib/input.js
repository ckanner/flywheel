import { realpathSync, statSync } from 'node:fs'
import { isAbsolute } from 'node:path'
import { createHash } from 'node:crypto'
import Ajv from 'ajv/dist/2020.js'

export class MachineInputError extends Error {
  constructor(message) { super('antigravity machine: ' + message); this.name = 'MachineInputError' }
}
export const object = value => value !== null && typeof value === 'object' && !Array.isArray(value)
export const hash = value => createHash('sha256').update(typeof value === 'string' ? value : JSON.stringify(value)).digest('hex')
export const UUID = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i
export function text(value, key, limit = 4096) {
  if (typeof value !== 'string' || !value.trim() || value.includes('\0') || Buffer.byteLength(value) > limit) throw new MachineInputError(key + ' must be nonempty bounded text without NUL')
  return value
}
export function integer(value, key, fallback, minimum = 1, maximum = 2147483647) {
  if (value === undefined) return fallback
  if (!Number.isSafeInteger(value) || value < minimum || value > maximum) throw new MachineInputError(key + ' is outside its integer range')
  return value
}
export function directory(value) {
  text(value, 'cwd')
  if (!isAbsolute(value) || !statSync(value).isDirectory()) throw new MachineInputError('cwd must be an existing absolute directory')
  return realpathSync(value)
}
export function fields(value, allowed, key) {
  if (!object(value) || Object.keys(value).some(k => !allowed.includes(k))) throw new MachineInputError(key + ' contains unsupported fields')
}
export function command(value = 'agy') {
  text(value, 'command')
  if (/\s/.test(value) && !isAbsolute(value) || value.startsWith('-')) throw new MachineInputError('command must be a program name or absolute path, never a shell command')
  return value
}
export function modelBinding(model, effort) {
  const match = /^gemini-4-(hydrogen|neon|carbon|argon)-(low|medium|high|xhigh)$/.exec(text(model, 'model', 256))
  if (!match || !['low', 'medium', 'high', 'xhigh'].includes(effort) || match[2] !== effort || ['carbon', 'argon'].includes(match[1]) && effort === 'xhigh') throw new MachineInputError('select an explicit Gemini 4 model slug with a supported matching effort')
  return { model, effort }
}
const ajv = new Ajv({ strict: true, allErrors: true, validateFormats: false })
export function schemaValidator(schema) {
  if (!object(schema) || Buffer.byteLength(JSON.stringify(schema)) > 1024 * 1024) throw new MachineInputError('outputSchema must be a JSON schema object up to 1 MiB')
  try { return ajv.compile(schema) } catch (error) { throw new MachineInputError('invalid outputSchema: ' + error.message) }
}
function freeze(value) {
  if (value && typeof value === 'object') { Object.values(value).forEach(freeze); Object.freeze(value) }
  return value
}
export function parseMachineRequest(input) {
  fields(input, ['protocol','requestId','cwd','prompt','command','model','effort','executionMode','permissionMode','sandbox','toolPolicy','sessionMode','sessionId','sessionName','timeoutMs','disposeGraceMs','maxOutputBytes','maxFrameBytes','outputSchema','subscriptionPolicy'], 'request')
  if (input.protocol !== 1) throw new MachineInputError('protocol must be 1')
  const { model, effort } = modelBinding(input.model, input.effort)
  if (!['default','accept-edits','plan'].includes(input.executionMode)) throw new MachineInputError('executionMode must be explicit: default, accept-edits or plan')
  if (!['request-review','always-proceed'].includes(input.permissionMode) || typeof input.sandbox !== 'boolean') throw new MachineInputError('permissionMode and sandbox must be explicit')
  if(input.toolPolicy!==undefined&&!['native','evidence-only'].includes(input.toolPolicy))throw new MachineInputError('toolPolicy must be native or evidence-only')
  if (!['new','resume'].includes(input.sessionMode)) throw new MachineInputError('sessionMode must be new or resume')
  if (input.sessionMode === 'resume' ? !UUID.test(input.sessionId) : input.sessionId !== undefined) throw new MachineInputError('resume requires an exact conversation UUID; new must not specify sessionId')
  if (input.sessionName !== undefined && /[\u0000-\u001f\u007f]/.test(text(input.sessionName, 'sessionName', 512))) throw new MachineInputError('sessionName must be a single line')
  fields(input.subscriptionPolicy, ['version','mode'], 'subscriptionPolicy')
  if (input.subscriptionPolicy.version !== 1 || input.subscriptionPolicy.mode !== 'subscription-only') throw new MachineInputError('subscriptionPolicy must select version 1 subscription-only')
  if (input.outputSchema !== undefined) schemaValidator(input.outputSchema)
  return freeze({ protocol:1, requestId:text(input.requestId,'requestId',256), cwd:directory(input.cwd), prompt:text(input.prompt,'prompt',1024*1024),
    command:command(input.command), model, effort, executionMode:input.executionMode, permissionMode:input.permissionMode, sandbox:input.sandbox,toolPolicy:input.toolPolicy??'native',
    sessionMode:input.sessionMode, ...(input.sessionId ? {sessionId:input.sessionId.toLowerCase()} : {}), ...(input.sessionName ? {sessionName:input.sessionName} : {}),
    timeoutMs:integer(input.timeoutMs,'timeoutMs',600000,0), disposeGraceMs:integer(input.disposeGraceMs,'disposeGraceMs',2000,10,30000),
    maxOutputBytes:integer(input.maxOutputBytes,'maxOutputBytes',1024*1024,1,16*1024*1024), maxFrameBytes:integer(input.maxFrameBytes,'maxFrameBytes',4*1024*1024,1024,32*1024*1024),
    ...(input.outputSchema === undefined ? {} : {outputSchema:structuredClone(input.outputSchema)}), subscriptionPolicy:{version:1,mode:'subscription-only'} })
}
