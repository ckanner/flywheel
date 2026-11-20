import z from '@deepseek-ai/schemastery'
import { command, fields, integer, modelBinding, text } from './input.js'
export const Config=z.object({
  command:z.string().volatile(),defaultCwd:z.string().volatile(),presets:z.array(z.string()).volatile(),
  model:z.string().volatile(),effort:z.union(['low','medium','high','xhigh']).volatile(),
  executionMode:z.union(['default','accept-edits','plan']).volatile(),permissionMode:z.union(['request-review','always-proceed']).volatile(),
  sandbox:z.boolean().volatile(),timeoutMs:z.number().min(0).volatile(),disposeGraceMs:z.number().min(10).volatile(),
})
export function resolveConfig(raw={}) {
  fields(raw,['command','defaultCwd','presets','model','effort','executionMode','permissionMode','sandbox','timeoutMs','disposeGraceMs'],'plugin configuration')
  const presets=raw.presets??['daily','heavy','cordis']
  if(!Array.isArray(presets)||presets.some(p=>typeof p!=='string'||!p))throw new Error('presets must contain preset IDs')
  const permissionMode=raw.permissionMode??'request-review',executionMode=raw.executionMode??'default',sandbox=raw.sandbox??false
  if(!['request-review','always-proceed'].includes(permissionMode)||!['default','accept-edits','plan'].includes(executionMode)||typeof sandbox!=='boolean')throw new Error('invalid native execution, permission or sandbox setting')
  return Object.freeze({command:command(raw.command),...(raw.defaultCwd?{defaultCwd:text(raw.defaultCwd,'defaultCwd')}:{}),presets:Object.freeze([...presets]),
    ...modelBinding(raw.model??'gemini-4-hydrogen-high',raw.effort??'high'),permissionMode,executionMode,sandbox,
    timeoutMs:integer(raw.timeoutMs,'timeoutMs',600000,0),disposeGraceMs:integer(raw.disposeGraceMs,'disposeGraceMs',2000,10,30000)})
}
