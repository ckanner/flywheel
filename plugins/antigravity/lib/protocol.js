import { realpathSync } from 'node:fs'
import { isDeepStrictEqual } from 'node:util'
import { object, schemaValidator, UUID } from './input.js'

/** An event parser for documented agy events, not Claude stream-json messages. */
export class EventParser {
  constructor(request) { this.request=request;this.init=null;this.sessionId=null;this.result=null;this.children=[];this.steps=0;this.permissionDenied=false;this.validate=request.outputSchema ? schemaValidator(request.outputSchema) : null }
  accept(event) {
    if (!object(event) || !['init','step_update','result'].includes(event.event)) throw new Error('unsupported or malformed agy event')
    if (this.result) throw new Error('received an event after the terminal result')
    if (event.event==='init') {
      if (this.init || !object(event.init) || typeof event.conversation_id!=='string' || !UUID.test(event.conversation_id)) throw new Error('missing or duplicate init identity')
      const init=event.init
      if (realpathSync(init.cwd)!==this.request.cwd || init.model!==this.request.model || init.permission_mode!==this.request.permissionMode) throw new Error('native cwd, model or permission mode did not match the request')
      if (this.request.sessionMode==='resume' && event.conversation_id.toLowerCase()!==this.request.sessionId) throw new Error('native conversation did not match the requested resume')
      if (this.request.outputSchema && !isDeepStrictEqual(init.json_schema,this.request.outputSchema)) throw new Error('native schema did not match the request')
      if(this.request.toolPolicy==='evidence-only'&&(init.agent!==this.request.evidenceAgent||!Array.isArray(init.tools)||init.tools.length!==0))throw new Error('evidence-only requires the pinned native agent and an observed empty tool list before prompt dispatch')
      this.init=init;this.sessionId=event.conversation_id
      return {kind:'init'}
    }
    if (!this.init) {
      if(event.event==='result'&&event.result?.status==='ERROR'&&event.result.conversation_id===''&&typeof event.result.response==='string'&&typeof event.result.error==='string'){
        this.result=event.result;return {kind:'result'}
      }
      throw new Error('agy emitted work before init')
    }
    if (event.event==='step_update') {
      const step=event.step_update
      if (!object(step)||step.conversation_id!==this.sessionId||!Number.isSafeInteger(step.step_index)||step.step_index<0||!['ACTIVE','DONE'].includes(step.state)) throw new Error('invalid step identity or state')
      this.steps++
      if(this.request.toolPolicy==='evidence-only'&&(step.step_type==='tool'||step.tool_name||step.tool_info||step.subagent_info))throw new Error('native evidence-only agent emitted a tool or subagent step')
      if (step.subagent_info?.subagents) {
        if (!Array.isArray(step.subagent_info.subagents)) throw new Error('invalid subagent metadata')
        for (const child of step.subagent_info.subagents) {
          if (!object(child)||typeof child.conversation_id!=='string'||!UUID.test(child.conversation_id)) throw new Error('invalid child identity')
          if(this.children.length>=1000)throw new Error('native child metadata exceeds retention limit')
          if (!this.children.some(c=>c.id===child.conversation_id)) this.children.push({id:child.conversation_id,kind:child.type_name??null,role:child.role??null,state:'unknown',model:null})
        }
      }
      const error=step.tool_info?.error
      if (error && /permission|approval|denied/i.test(String(error.type)+' '+String(error.message))) this.permissionDenied=true
      if (step.text_delta!==undefined && typeof step.text_delta!=='string') throw new Error('invalid text delta')
      return {kind:'step',text:step.step_type==='agent_response'?step.text_delta??'':'',child:!!step.subagent_info,accepted:true}
    }
    const result=event.result
    if (!object(result)||result.conversation_id!==this.sessionId||!['SUCCESS','ERROR','CANCELED','INTERRUPTED','INVALID','WAITING','RUNNING'].includes(result.status)||typeof result.response!=='string') throw new Error('invalid result identity or status')
    if (Buffer.byteLength(result.response)>this.request.maxOutputBytes) throw new Error('retained result exceeds maxOutputBytes')
    if (this.validate && result.status==='SUCCESS' && (!Object.hasOwn(result,'structured_output')||!this.validate(result.structured_output))) throw new Error('structured output does not satisfy the requested schema')
    this.result=result
    return {kind:'result'}
  }
}
export function usageOf(raw) {
  const fields={inputTokens:'input_tokens',outputTokens:'output_tokens',thinkingTokens:'thinking_tokens',cacheReadTokens:'cache_read_tokens',totalTokens:'total_tokens'}
  const out={status:'unknown',scope:object(raw)?'conversation':'unknown',cacheCreationTokens:null}
  for(const [to,from]of Object.entries(fields))out[to]=Number.isSafeInteger(raw?.[from])&&raw[from]>=0?raw[from]:null
  const observed=Object.keys(fields).filter(k=>out[k]!==null).length
  out.status=observed===5?'complete':observed?'partial':'unknown'
  return out
}
