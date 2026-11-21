import { mkdirSync, readdirSync, realpathSync } from 'node:fs'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import { runMachine, recoverMachine, doctor } from './machine.js'
import { directory, fields, hash, UUID, parseMachineRequest, modelEffort, text } from './input.js'
import { readRecord, writeRecord } from './journal.js'
import { runtimeEnvironment } from './environment.js'
import { nativeReceipt } from './native.js'

function taskSummary(record) {
  return {version:1,invocationId:record.invocationId,label:record.label,owner:record.owner,workspace:record.workspace,status:record.status,requested:record.requested,
    progress:record.progress,session:record.receipt?.session??null,configuration:record.receipt?.configuration??null,failure:record.receipt?.failure??record.error??null,
    cleanup:record.receipt?.cleanup??null,providerWork:record.receipt?.providerWork??null,native:record.receipt?.native??record.native??null,output:record.receipt?.output??[],updatedAt:record.updatedAt}
}
const selectedEffort=(input,config)=>input.effort??(input.model!==undefined&&input.model!==config.model?modelEffort(input.model):config.effort)
const selectedNative=(input,config)=>input.native===undefined?config.native:input.native
const activeKey=record=>hash({owner:record.owner,workspace:record.workspace,invocationId:record.invocationId})
/** Owner-bound direct actions. A manual task never constructs or calls a parent model. */
export class DesktopTasks {
  constructor({getConfig,env=process.env,run=runMachine}) {
    this.getConfig=getConfig;this.env=runtimeEnvironment({env});this.run=run;this.active=new Map();this.disposed=false
    this.root=join(this.env.HOME,'.local','state','dsh-subagent-antigravity','desktop')
    mkdirSync(this.root,{recursive:true,mode:0o700})
    if(realpathSync(this.root)!==this.root)throw new Error('desktop storage must be a physical directory')
  }
  scope(context) {
    const owner=text(context.owner,'owner',512),workspace=directory(context.workspace),path=join(this.root,hash({owner,workspace}))
    mkdirSync(path,{mode:0o700,recursive:true})
    return {owner,workspace,path}
  }
  record(context,id) {
    if(!UUID.test(id))throw new Error('invocationId must be an exact UUID')
    const scope=this.scope(context),record=readRecord(scope.path,id+'.json')
    if(record.owner!==scope.owner||record.workspace!==scope.workspace||record.invocationId!==id)throw new Error('task ownership mismatch')
    return {scope,record:this.active.get(activeKey(record))?.record??record}
  }
  persist(scope,record) {record.updatedAt=new Date().toISOString();writeRecord(scope.path,record.invocationId+'.json',record)}
  async action(context,input,{direct=true,signal,onProgress}={}) {
    fields(input,['version','action','invocationId','prompt','label','cwd','model','effort','native','executionMode','permissionMode','sandbox','toolPolicy','sessionId','timeoutMs','outputSchema'],'desktop action')
    if(input.version!==1||!['start','list','status','cancel','result','doctor','defaults'].includes(input.action))throw new Error('unsupported Antigravity action version or name')
    if(input.action==='defaults')return {version:1,defaults:this.getConfig()}
    if(input.action==='doctor'){
      const config=this.getConfig()
      return doctor({command:config.command,cwd:input.cwd??config.defaultCwd??context.workspace,model:input.model??config.model,effort:selectedEffort(input,config),native:selectedNative(input,config)},{env:this.env,signal})
    }
    if(input.action==='start')return this.start(context,input,{direct,signal,onProgress})
    if(input.action==='list'){
      const scope=this.scope(context),tasks=[]
      for(const name of readdirSync(scope.path).filter(n=>/^[0-9a-f-]{36}\.json$/.test(n)))tasks.push(await this.action(context,{version:1,action:'status',invocationId:name.slice(0,-5)}))
      return {version:1,tasks:tasks.sort((a,b)=>b.updatedAt.localeCompare(a.updatedAt))}
    }
    const {scope,record}=this.record(context,input.invocationId)
    if(input.action==='cancel'){
      const active=this.active.get(activeKey(record))
      if(active){active.controller.abort();record.status='cancelling';this.persist(scope,record)}
      else if(record.status==='running'||record.status==='cancelling'){record.status='unknown';record.error={category:'unknown',detail:'No live supervisor owned by this host. Read the original receipt; no name-based kill or replay.'};this.persist(scope,record)}
      return taskSummary(record)
    }
    if(!this.active.has(activeKey(record))&&['running','cancelling','unknown'].includes(record.status)){
      if(record.runId){const recovered=await recoverMachine({protocol:1,requestId:record.invocationId,runId:record.runId,recoveryDirectory:record.recoveryDirectory});if(recovered.receipt){record.receipt=recovered.receipt;record.status=record.receipt.ok?'completed':record.receipt.providerWork.state==='unknown'?'unknown':'failed'}}
      if(!record.receipt)record.status='unknown'
      this.persist(scope,record)
    }
    return input.action==='result'?{...taskSummary(record),receipt:record.receipt??null}:taskSummary(record)
  }
  start(context,input,{direct,signal,onProgress}) {
    if(this.disposed)throw new Error('Antigravity plugin is disposed')
    if(!direct&&['permissionMode','executionMode','sandbox','cwd'].some(k=>input[k]!==undefined))throw new Error('tool delegations cannot expand configured permissions or change their owner workspace')
    const config=this.getConfig(),scope=this.scope(context),invocationId=input.invocationId??randomUUID()
    if(!UUID.test(invocationId))throw new Error('invocationId must be an exact UUID')
    let resumeOwner
    if(input.sessionId){
      for(const file of readdirSync(scope.path).filter(n=>/^[0-9a-f-]{36}\.json$/.test(n))){
        const prior=readRecord(scope.path,file)
        if(prior.owner===scope.owner&&prior.workspace===scope.workspace&&prior.receipt?.session?.id===input.sessionId&&prior.receipt.session.resumable){resumeOwner=prior;break}
      }
      if(!resumeOwner)throw new Error('resume requires a confirmed conversation owned by this Desktop session and workspace')
    }
    // An absent native field is itself part of the exact session binding. Saved
    // defaults must not replace that absence when the owner resumes old work.
    const native=input.native===undefined&&resumeOwner?resumeOwner.requested.native:selectedNative(input,config)
    const request=parseMachineRequest({protocol:1,requestId:invocationId,command:config.command,cwd:direct?(input.cwd??config.defaultCwd??scope.workspace):scope.workspace,prompt:input.prompt,
      model:input.model??config.model,effort:selectedEffort(input,config),...(native===undefined?{}:{native}),executionMode:input.executionMode??config.executionMode,permissionMode:input.permissionMode??config.permissionMode,sandbox:input.sandbox??config.sandbox,
      toolPolicy:input.toolPolicy??'native',sessionMode:input.sessionId?'resume':'new',...(input.sessionId?{sessionId:input.sessionId}:{}),sessionName:input.label??'Antigravity task',
      timeoutMs:input.timeoutMs??config.timeoutMs,disposeGraceMs:config.disposeGraceMs,...(input.outputSchema?{outputSchema:input.outputSchema}:{}),subscriptionPolicy:{version:1,mode:'subscription-only'}})
    const digest=hash(request)
    try{const existing=readRecord(scope.path,invocationId+'.json');if(existing.digest!==digest)throw new Error('invocation identity conflicts with a previous task');return taskSummary(existing)}catch(error){if(error.code!=='ENOENT')throw error}
    // Durable identity is recorded before invoking machine; retries of the same action ID are reads.
    const {prompt,outputSchema,...requested}=request
    const record={version:1,invocationId,owner:scope.owner,workspace:scope.workspace,label:request.sessionName,requested,digest,status:'running',progress:'',native:nativeReceipt(request),runId:null,recoveryDirectory:join(scope.path,invocationId+'.journal'),receipt:null}
    this.persist(scope,record)
    const controller=new AbortController(),abort=()=>controller.abort()
    signal?.addEventListener('abort',abort,{once:true});if(signal?.aborted)abort()
    const active={controller,record,done:null};this.active.set(activeKey(record),active)
    active.done=this.run(request,{env:this.env,signal:controller.signal,recoveryDirectory:record.recoveryDirectory,
      onLifecycle:event=>{record.runId=event.runId;record.lifecycle=event;if(event.event==='dispatch-possible')record.native.completion.state='unknown';this.persist(scope,record)},
      onActivity:event=>{if(event.native)record.native=structuredClone(event.native)},
      onProgress:value=>{record.progress=(record.progress+value).slice(-4096);try{onProgress?.(value)}catch{}},
    }).then(receipt=>{record.receipt=receipt;record.status=receipt.ok?'completed':receipt.providerWork.state==='unknown'?'unknown':'failed';return receipt},error=>{record.status='unknown';record.error={category:'unknown',detail:error.message};return null}).finally(()=>{
      signal?.removeEventListener('abort',abort);this.active.delete(activeKey(record));this.persist(scope,record)
    })
    return taskSummary(record)
  }
  async wait(context,id) {
    const {record}=this.record(context,id)
    await this.active.get(activeKey(record))?.done
    return this.action(context,{version:1,action:'result',invocationId:id})
  }
  async dispose() {this.disposed=true;const active=[...this.active.values()];for(const run of active)run.controller.abort();await Promise.allSettled(active.map(run=>run.done))}
}
