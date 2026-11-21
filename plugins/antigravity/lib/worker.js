import { runLocal, baseReceipt } from './runner.js'
import { appendLifecycle, finishJournal } from './journal.js'
import { redact } from './environment.js'

const controller=new AbortController(),pending=new Map()
let started=false,parentGone=false,sequence=0
const send=message=>new Promise(resolve=>{if(!process.connected)return resolve();process.send(message,()=>resolve())})
const cancel=()=>controller.abort()
process.on('disconnect',()=>{parentGone=true;cancel();for(const value of pending.values())value.reject(new Error('parent disconnected'));pending.clear()})
process.on('SIGTERM',cancel);process.on('SIGINT',cancel)
process.on('message',async message=>{
  if(message?.type==='cancel'){cancel();return}
  if(message?.type==='ack'){
    const item=pending.get(message.sequence)
    if(item){pending.delete(message.sequence);message.ok?item.resolve():item.reject(new Error('lifecycle acknowledgement rejected: '+message.error))}
    return
  }
  if(message?.type!=='start'||started)return
  started=true
  const {request,runId,journal}=message
  const emit=async(event,receipt)=>{
    const terminal=event==='terminal'||event==='cleanup-confirmed'
    const value={lifecycleVersion:1,type:'lifecycle',requestId:request.requestId,runId,sequence:++sequence,event,
      threadId:receipt.session.id,turnId:null,persistent:true,configuration:event==='dispatch-intent'?null:receipt.configuration,
      dispatch:event==='dispatch-possible'?'possible':receipt.execution.turnDispatch,
      outcome:terminal?(receipt.execution.outcome==='not-started'?'failed':receipt.execution.outcome):'unknown',
      cleanupConfirmed:terminal&&receipt.cleanup.confirmed}
    appendLifecycle(journal,value)
    if(parentGone)return
    await new Promise((resolve,reject)=>{
      const timer=setTimeout(()=>{pending.delete(value.sequence);reject(new Error('lifecycle acknowledgement deadline exceeded'))},10000)
      pending.set(value.sequence,{resolve:()=>{clearTimeout(timer);resolve()},reject:error=>{clearTimeout(timer);reject(error)}})
      void send({type:'lifecycle',event:value})
    })
  }
  let receipt
  try{receipt=await runLocal(request,{env:process.env,signal:controller.signal,recoveryDirectory:journal,onActivity:event=>void send({type:'activity',event}),onProgress:text=>void send({type:'progress',text})},runId,emit)}
  catch(error){receipt=baseReceipt(request,runId);receipt.cleanup.confirmed=false;receipt.execution.outcome='unknown';receipt.providerWork={...receipt.providerWork,state:'unknown',replaySafe:false};receipt.failure={stage:'supervisor',category:'unknown',detail:redact(error.message)}}
  try{
    // Save the receipt before advisory terminal notifications; a lost caller can recover it.
    receipt.recovery={protocol:1,requestId:request.requestId,runId,recoveryDirectory:journal}
    finishJournal(journal,receipt)
    // This is the local invocation terminal checkpoint. Provider completion is
    // independently represented by outcome and the immutable native receipt.
    await emit('terminal',receipt)
    if(receipt.cleanup.confirmed)await emit('cleanup-confirmed',receipt)
  }catch(error){receipt.ok=false;receipt.failure??={stage:'journal',category:'lifecycle',detail:redact(error.message)};try{finishJournal(journal,receipt)}catch{}}
  await send({type:'result',receipt})
  if(process.connected)process.disconnect()
  process.exitCode=receipt.ok?0:1
})
