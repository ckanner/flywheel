import { fork } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { randomUUID } from 'node:crypto'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { parseMachineRequest, MachineInputError } from './input.js'
import { runtimeEnvironment, redact } from './environment.js'
import { createJournal, recoverMachine } from './journal.js'
import { baseReceipt } from './runner.js'
export { parseMachineRequest, MachineInputError, recoverMachine }
export { doctor } from './probe.js'
export const MACHINE_PROTOCOL_VERSION=1
export const MACHINE_LIFECYCLE_VERSION=1
export const MACHINE_ACTIVITY_VERSION=1
export const MACHINE_LIFECYCLE_CAPABILITIES=Object.freeze({version:1,durableJournal:true,readOnlyRecovery:true,threadReadyBeforeDispatch:false})
export const MACHINE_ACTIVITY_CAPABILITIES=Object.freeze({version:1,metadataOnly:true,rootAndChildren:true})
export const MACHINE_CAPABILITIES=Object.freeze({protocol:1,provider:'antigravity',transport:'official-cli-stream-json',minimumCLIVersion:'1.5.7',subscriptionOnly:true,
  platforms:['darwin','linux'],input:'text',structuredOutput:true,preciseResume:true,concurrentResume:false,parentDeathCleanup:true,processGroupCleanup:true,
  durableRecovery:true,providerWorkQuery:false,providerWorkCancel:false,prewarm:false,nativeRename:false,evidenceOnly:'verified-empty-init-tools',nativeAttachments:false,
  nativeOrchestrator:false,aggregateNativeUsage:false,observedBinding:['cwd','model','permissionMode'],unobservedBinding:['effort','executionMode','sandbox']})

/** One isolated supervisor and one official CLI turn. No model calls during validation. */
export async function runMachine(input,runtime={}) {
  const request=parseMachineRequest(input),env=runtimeEnvironment(runtime),runId=randomUUID()
  for(const key of ['onProgress','onActivity','onLifecycle'])if(runtime[key]!==undefined&&typeof runtime[key]!=='function')throw new MachineInputError(key+' must be a function')
  if(runtime.signal!==undefined&&!(runtime.signal instanceof AbortSignal))throw new MachineInputError('signal must be an AbortSignal')
  let recoveryDirectory=runtime.recoveryDirectory
  if(recoveryDirectory===undefined){const root=join(env.HOME,'.local','state','dsh-subagent-antigravity','runs');mkdirSync(root,{recursive:true,mode:0o700});recoveryDirectory=join(root,runId)}
  const journal=createJournal(recoveryDirectory,request,runId)
  return new Promise((resolve,reject)=>{
    let receipt=null,lifecycle=null,settled=false,stderr='',callbackQueue=Promise.resolve()
    const worker=fork(fileURLToPath(new URL('./worker.js',import.meta.url)),[],{cwd:request.cwd,env,execArgv:[],detached:true,serialization:'json',stdio:['ignore','ignore','pipe','ipc']})
    const send=message=>{if(worker.connected)worker.send(message,()=>{})}
    const abort=()=>send({type:'cancel'})
    const finish=async()=>{
      if(settled)return
      settled=true;runtime.signal?.removeEventListener('abort',abort)
      if(!receipt&&journal){const recovered=await recoverMachine({protocol:1,requestId:request.requestId,runId,recoveryDirectory:journal});receipt=recovered.receipt}
      if(!receipt){
        receipt=baseReceipt(request,runId)
        const possible=lifecycle?.dispatch!=='not-dispatched'&&lifecycle!==null
        receipt.execution.dispatch=possible?'dispatched':'not-dispatched';receipt.execution.turnDispatch=possible?'possible':'not-dispatched'
        receipt.execution.outcome=possible?'unknown':'not-started';receipt.cleanup.confirmed=!possible
        receipt.providerWork={...receipt.providerWork,state:possible?'unknown':'not-dispatched',replaySafe:!possible}
        receipt.failure={stage:'supervisor',category:'unknown',detail:'Supervisor exited without a verified receipt. '+stderr.slice(-2000)}
      }
      receipt.recovery={protocol:1,requestId:request.requestId,runId,recoveryDirectory:journal}
      resolve(receipt)
    }
    worker.stderr.on('data',b=>{stderr=(stderr+redact(b.toString('utf8'))).slice(-8192)})
    worker.on('message',message=>{
      if(message?.type==='lifecycle'){
        lifecycle=message.event
        callbackQueue=callbackQueue.then(async()=>{
          try{await runtime.onLifecycle?.(message.event);send({type:'ack',sequence:message.event.sequence,ok:true})}
          catch(error){send({type:'ack',sequence:message.event.sequence,ok:false,error:redact(error.message)})}
        })
      }else if(message?.type==='activity'){try{runtime.onActivity?.(message.event)}catch{/* Advisory callbacks cannot change native outcome. */}}
      else if(message?.type==='progress'){try{runtime.onProgress?.(message.text)}catch{/* Same advisory boundary. */}}
      else if(message?.type==='result')receipt=message.receipt
    })
    worker.on('error',error=>{stderr=redact(error.message)})
    worker.once('close',()=>{void finish().catch(reject)})
    worker.once('spawn',()=>{send({type:'start',request,runId,journal});if(runtime.signal?.aborted)abort()})
    runtime.signal?.addEventListener('abort',abort,{once:true})
  })
}
