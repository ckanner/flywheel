import { mkdirSync, lstatSync, realpathSync, openSync, closeSync, constants, unlinkSync } from 'node:fs'
import { join } from 'node:path'
import { hash } from './input.js'
import { readRecord, writeRecord } from './journal.js'

function storage(env) {
  const path=join(env.HOME,'.local','state','dsh-subagent-antigravity','sessions')
  mkdirSync(path,{recursive:true,mode:0o700})
  const stat=lstatSync(path)
  if(realpathSync(path)!==path||!stat.isDirectory()||(stat.mode&0o077)!==0||process.getuid&&stat.uid!==process.getuid())throw new Error('session storage must be a private owned physical directory')
  return path
}
export function sessionBinding(request,fingerprint) {
  return hash({cwd:request.cwd,command:request.command,model:request.model,effort:request.effort,permissionMode:request.permissionMode,executionMode:request.executionMode,sandbox:request.sandbox,toolPolicy:request.toolPolicy,configuration:fingerprint})
}
export function claimSession(env,id,binding,runId,{resume=false}={}) {
  const directory=storage(env),key=hash(id.toLowerCase()),lock=join(directory,key+'.lock')
  let fd
  try{fd=openSync(lock,constants.O_CREAT|constants.O_EXCL|constants.O_WRONLY|constants.O_NOFOLLOW,0o600)}catch(error){if(error.code==='EEXIST')throw new Error('conversation is active or unresolved; inspect its original invocation before resuming');throw error}
  closeSync(fd)
  try {
    let prior
    try{prior=readRecord(directory,key+'.json')}catch(error){if(error.code!=='ENOENT')throw error}
    if(resume&&(!prior||prior.status!=='terminal'||prior.binding!==binding))throw new Error('resume requires a confirmed adapter-owned conversation with identical cwd, model, effort, permissions and native settings')
    if(!resume&&prior)throw new Error('new invocation reused an existing native conversation')
    writeRecord(directory,key+'.json',{id,binding,runId,status:'active'})
    let released=false
    return {finish(receipt){
      if(released)return
      const confirmed=receipt.execution.terminalReceived&&receipt.cleanup.confirmed
      const notDispatched=receipt.execution.dispatch==='not-dispatched'
      writeRecord(directory,key+'.json',{id,binding,runId,status:confirmed?'terminal':notDispatched&&prior?'terminal':'unknown'})
      if(confirmed||notDispatched){unlinkSync(lock);released=true}
    }}
  }catch(error){unlinkSync(lock);throw error}
}
