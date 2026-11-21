import { spawn } from 'node:child_process'
import { StringDecoder } from 'node:string_decoder'
import { EventParser, usageOf } from './protocol.js'
import { inspectSubscription, redact } from './environment.js'
import { inspectCLI } from './probe.js'
import { sessionBinding, claimSession } from './sessions.js'
import { prepareEvidenceAgent } from './evidence-agent.js'
import { writeRecord } from './journal.js'
import { nativeReceipt, observeChildren, promptFor } from './native.js'

export function baseReceipt(request,runId) {
  const {prompt,outputSchema,...requested}=request
  return {protocol:1,type:'result',requestId:request.requestId,runId,ok:false,requested,
    execution:{pid:null,processGroupId:null,ownership:'none',dispatch:'not-dispatched',turnDispatch:'not-dispatched',outcome:'not-started',terminalReceived:false,finishedReason:null,exitCode:null,signal:null,cancelled:false,timedOut:false,truncated:false},
    cleanup:{confirmed:true,method:'not-started'},providerWork:{state:'not-dispatched',querySupported:false,cancelSupported:false,replaySafe:true},
    session:{mode:request.sessionMode,id:null,resumed:request.sessionMode==='resume',resumable:false,name:request.sessionName??null,nativeNameStatus:'unknown'},
    configuration:{source:null,observed:null,matched:false,fingerprint:null},
    subscription:{mode:'subscription-only',status:'unknown',authentication:'native-account-unverified',creditsFallback:false,extraBilling:'unknown',configFingerprint:null},
    output:[],usage:usageOf(null),native:nativeReceipt(request),reportedCostUSD:null,costUSD:null,costStatus:'unknown',failure:null}
}
export function argumentsFor(r,agent) {
  const args=['--input-format','stream-json','--output-format','stream-json','--model',r.model,'--mode',r.executionMode==='default'?'':r.executionMode,'--print-timeout',r.timeoutMs+'ms']
  if(r.effort!=='')args.push('--effort',r.effort)
  if(r.native?.mode!=='swarm')args.push('--disable-slash-commands')
  if(r.permissionMode==='always-proceed')args.push('--dangerously-skip-permissions')
  if(r.sandbox)args.push('--sandbox')
  if(r.sessionMode==='resume')args.push('--conversation',r.sessionId)
  if(r.outputSchema)args.push('--json-schema',JSON.stringify(r.outputSchema))
  if(agent)args.push('--agent',agent)
  return args
}
function groupExists(pid){try{process.kill(-pid,0);return true}catch(e){return e.code!=='ESRCH'}}
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms))
export async function stopGroup(pid,grace) {
  if(!pid)return true
  try{process.kill(-pid,'SIGTERM')}catch(e){if(e.code==='ESRCH')return true}
  const end=Date.now()+grace
  while(groupExists(pid)&&Date.now()<end)await delay(20)
  if(groupExists(pid))try{process.kill(-pid,'SIGKILL')}catch{}
  const killedEnd=Date.now()+Math.max(grace,1000)
  while(groupExists(pid)&&Date.now()<killedEnd)await delay(20)
  return !groupExists(pid)
}
export async function runLocal(request,runtime,runId,emit) {
  const receipt=baseReceipt(request,runId),signal=runtime.signal,started=Date.now()
  const fail=(stage,category,detail)=>{receipt.failure??={stage,category,detail:redact(detail)}}
  if(process.platform==='win32'){fail('start','unsupported-runtime','process-group supervision currently supports macOS/Linux');return receipt}
  if(signal.aborted){receipt.execution.cancelled=true;fail('start','cancelled','cancelled before dispatch');return receipt}
  let cli,subscription,lease,evidenceAgent,binding
  try {
    subscription=inspectSubscription(runtime.env,request)
    cli=await inspectCLI(request,runtime.env,signal)
    if(inspectSubscription(runtime.env,request).fingerprint!==subscription.fingerprint)throw new Error('native settings changed during preflight')
    receipt.configuration.fingerprint=subscription.fingerprint
    receipt.subscription={...receipt.subscription,...subscription,configFingerprint:subscription.fingerprint}
    binding=sessionBinding(request,subscription.fingerprint)
    if(request.sessionMode==='resume')lease=claimSession(runtime.env,request.sessionId,binding,runId,{resume:true})
    if(request.toolPolicy==='evidence-only'){
      evidenceAgent=prepareEvidenceAgent(request.cwd,runId)
      const {name,path,digest,ownerRunId}=evidenceAgent
      receipt.native.managedAgent={name,path,digest,ownerRunId,cleanup:'unknown'}
      if(runtime.recoveryDirectory)writeRecord(runtime.recoveryDirectory,'managed-agent.json',receipt.native.managedAgent)
    }
    await emit('dispatch-intent',receipt)
  } catch(error){fail('configuration','invalid-request',error.message)}
  if(signal.aborted||request.timeoutMs>0&&Date.now()-started>=request.timeoutMs){receipt.execution.cancelled=signal.aborted;receipt.execution.timedOut=!signal.aborted;fail('start',signal.aborted?'cancelled':'timeout','stopped during preflight')}
  if(receipt.failure){try{lease?.finish(receipt);evidenceAgent?.dispose();if(evidenceAgent)receipt.native.managedAgent.cleanup='confirmed'}catch(error){fail('cleanup','cleanup-unconfirmed',error.message);receipt.cleanup.confirmed=false}return receipt}
  const parser=new EventParser({...request,evidenceAgent:evidenceAgent?.name}),decoder=new StringDecoder('utf8')
  let buffer='',stderr='',closed=false,stopPromise=null,child,closeResolve,promptSent=false,dispatching=false,activitySequence=0,protocolFailed=false
  const closePromise=new Promise(resolve=>{closeResolve=resolve})
  const stop=()=>stopPromise??=(async()=>{
    const clean=await stopGroup(child?.pid,request.disposeGraceMs)
    if(!closed){child?.stdout?.destroy();child?.stderr?.destroy();child?.stdin?.destroy()}
    receipt.cleanup.confirmed=clean
    return clean
  })()
  const abort=()=>{receipt.execution.cancelled=true;fail('run','cancelled','invocation cancelled');void stop()}
  const dispatchPrompt=async()=>{
    if(promptSent||dispatching)return
    dispatching=true
    try {
      evidenceAgent?.verify()
      // Durable possible-dispatch evidence and caller acknowledgement precede stdin.
      await emit('dispatch-possible',receipt)
      if(signal.aborted||receipt.failure||closed)return
      receipt.execution.dispatch='dispatched';receipt.execution.turnDispatch='possible';receipt.providerWork={...receipt.providerWork,state:'unknown',replaySafe:false};receipt.native.completion.state='unknown'
      promptSent=true
      child.stdin.end(JSON.stringify({event:'user',message:{content:promptFor(request)}})+'\n')
    }catch(error){fail('start','lifecycle',error.message);void stop()}
  }
  const handleLine=line=>{
    if(!line.trim())return
    if(Buffer.byteLength(line)>request.maxFrameBytes)throw new Error('NDJSON frame exceeds maxFrameBytes')
    const event=parser.accept(JSON.parse(line))
    if(event.kind==='init'){
      receipt.configuration={...receipt.configuration,source:'cli-init',matched:true,observed:{cwd:parser.init.cwd,model:parser.init.model,permissionMode:parser.init.permission_mode,agent:parser.init.agent??null,tools:Array.isArray(parser.init.tools)?parser.init.tools:null,cliVersion:cli.version,effort:null,executionMode:null,sandbox:null}}
      receipt.session.id=parser.sessionId
      if(inspectSubscription(runtime.env,request).fingerprint!==subscription.fingerprint)throw new Error('native settings changed before accepted init')
      if(!lease)lease=claimSession(runtime.env,parser.sessionId,binding,runId)
      if(evidenceAgent||request.native&&request.native.mode!=='native')void dispatchPrompt()
    }
    if(event.accepted){if(!promptSent)throw new Error('native work appeared before prompt dispatch');receipt.execution.turnDispatch='accepted'}
    if(event.child)observeChildren(receipt.native,parser.children,parser.childMetadataTruncated)
    try{runtime.onActivity?.({version:1,sequence:++activitySequence,kind:'provider-progress',source:'antigravity',scope:event.child?'child':'root',eventTypes:[event.kind],observedEvents:1,...(event.child?{native:structuredClone(receipt.native)}:{})})}catch{}
    if(receipt.native.limit.exceeded){fail('run','observed-agent-limit','observed unique child count exceeded native.maxObservedAgents; this is not a native concurrency cap');void stop()}
    if(event.text)try{runtime.onProgress?.(event.text)}catch{}
  }
  try {
    child=spawn(request.command,argumentsFor(request,evidenceAgent?.name),{cwd:request.cwd,env:runtime.env,shell:false,detached:true,windowsHide:true,stdio:['pipe','pipe','pipe']})
    receipt.execution.pid=child.pid??null;receipt.execution.processGroupId=child.pid??null
    receipt.execution.ownership=child.pid?'process-group':'none';receipt.cleanup={confirmed:false,method:'process-group'}
    child.once('spawn',()=>{
      if(!evidenceAgent&&(!request.native||request.native.mode==='native'))void dispatchPrompt()
    })
    child.stdin.on('error',error=>{fail('run','transport',error.message);void stop()})
    child.stdout.on('data',chunk=>{
      if(protocolFailed)return
      try {
        buffer+=decoder.write(chunk)
        let at
        while((at=buffer.indexOf('\n'))>=0){const line=buffer.slice(0,at);buffer=buffer.slice(at+1);handleLine(line)}
        if(Buffer.byteLength(buffer)>request.maxFrameBytes)throw new Error('incomplete NDJSON frame exceeds maxFrameBytes')
      }catch(error){protocolFailed=true;receipt.execution.truncated=/exceeds/.test(error.message);fail('run','protocol',error.message);void stop()}
    })
    child.stderr.on('data',chunk=>{
      stderr=(stderr+redact(chunk.toString('utf8'))).slice(-65536)
      if(/soft.denied|permission.*denied|requires?.*approval|approval.*required/i.test(stderr))parser.permissionDenied=true
    })
    child.on('error',error=>{fail('start','spawn',error.message);receipt.cleanup={confirmed:true,method:'not-started'}})
    child.once('close',(code,sig)=>{closed=true;receipt.execution.exitCode=code;receipt.execution.signal=sig;closeResolve()})
    signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort()
    const remaining=request.timeoutMs>0?Math.max(1,request.timeoutMs-(Date.now()-started)):0
    const timer=remaining?setTimeout(()=>{receipt.execution.timedOut=true;fail('run','timeout','total invocation deadline exceeded');void stop()},remaining):null
    await closePromise
    if(timer)clearTimeout(timer)
    if(stopPromise)await stopPromise
    else receipt.cleanup.confirmed=await stopGroup(child.pid,request.disposeGraceMs)
    if(!receipt.failure){buffer+=decoder.end();if(buffer.trim())handleLine(buffer)}
  }catch(error){protocolFailed=true;fail('run','protocol',error.message);await stop()}
  finally{signal.removeEventListener('abort',abort)}
  const result=parser.result
  observeChildren(receipt.native,parser.children,parser.childMetadataTruncated)
  if(result){
    receipt.execution.terminalReceived=!['WAITING','RUNNING'].includes(result.status)
    receipt.execution.finishedReason=result.finished_reason||null
    receipt.output=[{type:'text',text:result.response}];receipt.usage=usageOf(result.usage)
    if(Object.hasOwn(result,'structured_output'))receipt.structured=result.structured_output
    const completedNormally=result.status==='SUCCESS'&&!result.finished_reason
    if(receipt.execution.terminalReceived)receipt.providerWork={...receipt.providerWork,state:completedNormally?'completed':'failed',replaySafe:true}
    if(result.finished_reason)fail('run','native-cutoff','native invocation ended with finished_reason: '+result.finished_reason)
    if(result.status!=='SUCCESS'){
      const error=result.error??result.status
      const category=/auth|sign.?in|log.?in/i.test(error)?'auth':/quota|exhaust/i.test(error)?'quota':/rate.limit|too many requests/i.test(error)?'rate-limit':/model.*invalid|invalid.*model/i.test(error)?'invalid-request':'product-error'
      fail('run',category,error)
    }
  }else fail('run','unknown','no native terminal result received; do not replay')
  if(parser.permissionDenied)fail('run','permission-denied','a required tool action was denied by native policy')
  if(receipt.execution.exitCode!==0)fail('process','process','CLI did not exit successfully')
  if(!receipt.configuration.matched)fail('configuration','configuration-mismatch','native configuration was not confirmed')
  try{if(inspectSubscription(runtime.env,request).fingerprint!==subscription.fingerprint)fail('configuration','configuration-mismatch','native settings changed during the invocation')}catch(error){fail('configuration','configuration-mismatch',error.message)}
  if(!receipt.cleanup.confirmed)fail('cleanup','cleanup-unconfirmed','local process group cleanup is not confirmed')
  const nativeCompleted=promptSent&&!protocolFailed&&receipt.execution.terminalReceived&&result?.status==='SUCCESS'&&!result.finished_reason&&receipt.execution.exitCode===0
  receipt.native.completion={scope:'cli-invocation',state:nativeCompleted?'completed':!promptSent?'not-started':receipt.execution.terminalReceived?'failed':'unknown',source:nativeCompleted?'normal-result-and-cli-exit':null,children:'unknown'}
  if(nativeCompleted&&request.native&&request.native.mode!=='native'&&!receipt.native.collaborationObserved)fail('run','collaboration-not-observed','requested '+request.native.mode+' completed without child metadata; native collaboration was not verified')
  try{lease?.finish(receipt);evidenceAgent?.dispose();if(evidenceAgent)receipt.native.managedAgent.cleanup='confirmed'}catch(error){fail('cleanup','cleanup-unconfirmed',error.message);receipt.cleanup.confirmed=false}
  receipt.ok=!receipt.failure&&promptSent&&result?.status==='SUCCESS'&&receipt.execution.terminalReceived&&receipt.cleanup.confirmed
  receipt.execution.outcome=receipt.ok?'completed':receipt.execution.dispatch==='not-dispatched'?'not-started':receipt.execution.terminalReceived?'failed':'unknown'
  receipt.session.resumable=!!receipt.session.id&&receipt.execution.terminalReceived&&receipt.cleanup.confirmed
  return receipt
}
