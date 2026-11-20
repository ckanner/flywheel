import { Config, resolveConfig } from './config.js'
import { DesktopTasks } from './desktop.js'
export { Config, resolveConfig }
export const name='antigravity'
export const inject=['commands','workingDirectory']
const toolOutput={oneOf:[{type:'object',properties:{content:{type:'array',items:{type:'object',properties:{type:{type:'string',const:'text'},text:{type:'string'}},required:['type','text'],additionalProperties:false}}},required:['content'],additionalProperties:false},{type:'object',properties:{kind:{type:'string',const:'background'},jobId:{type:'string'}},required:['kind','jobId'],additionalProperties:false}]}
const summary=result=>JSON.stringify({invocationId:result.invocationId,status:result.status,session:result.session,output:result.output,failure:result.failure,cleanup:result.cleanup,providerWork:result.providerWork,native:result.native})
const nativeSchema={type:'object',additionalProperties:false,properties:{version:{type:'integer',const:1},mode:{type:'string',enum:['native','subagents','swarm']},maxObservedAgents:{type:'integer',enum:[1,2,3,4,5,6,7,8],description:'Stop when the observed total of unique child IDs exceeds this value, including verifier and monitor. Not a hard native concurrency limit.'}},required:['version','mode']}

export function apply(ctx,rawConfig={}) {
  resolveConfig(rawConfig)
  if(typeof ctx.commands?.register!=='function'||typeof ctx.workingDirectory?.get!=='function'||typeof ctx.on!=='function')throw new Error('Antigravity requires DSH commands, workingDirectory and Agent lifecycle services')
  const tasks=new DesktopTasks({getConfig:()=>resolveConfig(rawConfig)}),jobs=ctx.get?.('jobs')
  const owner=agent=>({owner:String(agent.session.id??agent.id),workspace:ctx.workingDirectory.get(agent.session)??agent.session.header?.cwd})
  const allowed=agent=>{
    const registry=ctx.get?.('agentPresets')
    let preset
    try{preset=registry?.composedPreset?.(agent.ctx)??agent.session.header?.agentPreset}catch{return false}
    return resolveConfig(rawConfig).presets.includes(preset)
  }
  const serve=agent=>{
    if(!agent||!allowed(agent)||typeof agent.ctx?.tools?.register!=='function')return
    agent.ctx.tools.register({name:'subagent_antigravity',description:'Delegate a complete text task to the official Antigravity CLI using its native subscription. Choose native behavior, explicit subagents, or Swarm with native.version 1. An Orchestrator model uses empty effort; adjustable model slug and effort must match. Native collaboration completion covers the CLI invocation; child states and aggregate usage can remain unknown. Read the receipt; unknown results must not be replayed. Background jobs use job_output and job_kill. An exact sessionId resumes only a confirmed matching conversation.',
      parameters:{type:'object',additionalProperties:false,properties:{prompt:{type:'string'},label:{type:'string'},model:{type:'string'},effort:{type:'string',enum:['','low','medium','high','xhigh']},native:nativeSchema,sessionId:{type:'string'},toolPolicy:{type:'string',enum:['native','evidence-only']},timeoutMs:{type:'integer',description:'Total deadline in milliseconds; 0 is unlimited.'},outputSchema:{type:'object'},run_in_background:{type:'boolean'}},required:['prompt']},
      output:{schema:toolOutput,render:(_args,value)=>value.content??[{type:'text',text:'Antigravity background job '+value.jobId}]},
      async execute(args,execution){
        if(!allowed(agent))throw new Error('Antigravity is not enabled for the current committed preset')
        const {run_in_background,...call}=args
        const context=owner(agent)
        const run=async(signal,onProgress)=>{const started=await tasks.action(context,{version:1,action:'start',...call},{direct:false,signal,onProgress});return summary(await tasks.wait(context,started.invocationId))}
        if(run_in_background){
          if(!jobs?.start)throw new Error('Background Antigravity delegation requires the DSH jobs service')
          const controller=new AbortController()
          const jobId=jobs.start({kind:'subagent',label:call.label??'Antigravity task',owner:String(agent.id),run:handle=>({cancel:()=>controller.abort(),done:run(controller.signal,t=>handle.append(t)).then(result=>({status:controller.signal.aborted?'killed':JSON.parse(result).status==='completed'?'completed':'failed',result}),error=>({status:'failed',result:error.message}))})})
          return {kind:'background',jobId}
        }
        return {content:[{type:'text',text:await run(execution?.signal)}]}
      }})
  }
  ctx.commands.register({name:'antigravity',description:'Run or inspect an Antigravity task directly without invoking the parent model.',input:{hint:'JSON action (version 1): start, list, status, cancel, result, doctor'},async handler({rawInput,agent,signal}){
    try{const result=await tasks.action(owner(agent),JSON.parse(rawInput),{direct:true,signal});return {kind:'success',text:JSON.stringify(result)}}catch(error){return {kind:'error',text:error.message}}
  }})
  ctx.on('agent/created',({agent})=>serve(agent))
  ctx.on('agent-preset/selected',(sessionId)=>serve(ctx.get?.('agents')?.get?.(String(sessionId))))
  ctx.on('dispose',()=>tasks.dispose())
}
