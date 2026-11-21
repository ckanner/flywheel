import { constants, closeSync, fsyncSync, fstatSync, lstatSync, mkdirSync, openSync, readFileSync, realpathSync, renameSync, writeFileSync } from 'node:fs'
import { dirname, join, isAbsolute } from 'node:path'
import { randomUUID } from 'node:crypto'
import { fields, text, UUID, hash } from './input.js'

function privateDirectory(path) {
  const st=lstatSync(path)
  if (!isAbsolute(path)||!st.isDirectory()||st.isSymbolicLink()||realpathSync(path)!==path||(st.mode&0o077)!==0||process.getuid&&st.uid!==process.getuid()) throw new Error('recovery storage must be a private owned physical directory')
}
export function writeRecord(directory,name,value) {
  privateDirectory(directory)
  const temp=join(directory,'.write-'+randomUUID())
  const fd=openSync(temp,constants.O_WRONLY|constants.O_CREAT|constants.O_EXCL|constants.O_NOFOLLOW,0o600)
  try {writeFileSync(fd,JSON.stringify(value)+'\n');fsyncSync(fd)} finally {closeSync(fd)}
  renameSync(temp,join(directory,name))
  const d=openSync(directory,constants.O_RDONLY|constants.O_DIRECTORY)
  try{fsyncSync(d)}finally{closeSync(d)}
}
export function readRecord(directory,name) {
  privateDirectory(directory)
  const fd=openSync(join(directory,name),constants.O_RDONLY|constants.O_NOFOLLOW)
  try {
    const st=fstatSync(fd)
    if(!st.isFile()||st.size>40*1024*1024||(st.mode&0o077)!==0||process.getuid&&st.uid!==process.getuid())throw new Error('untrusted recovery record')
    return JSON.parse(readFileSync(fd,'utf8'))
  }finally{closeSync(fd)}
}
export function createJournal(directory,request,runId) {
  if (!directory) return null
  privateDirectory(dirname(directory))
  mkdirSync(directory,{mode:0o700})
  writeRecord(directory,'identity.json',{protocol:1,requestId:request.requestId,runId,requestDigest:hash(request),createdAt:new Date().toISOString()})
  return directory
}
export function appendLifecycle(directory,event) {if(directory)writeRecord(directory,'lifecycle.json',event)}
export function finishJournal(directory,receipt) {if(directory)writeRecord(directory,'receipt.json',receipt)}
export async function recoverMachine(input) {
  fields(input,['protocol','requestId','runId','recoveryDirectory'],'recovery request')
  if(input.protocol!==1||!UUID.test(input.runId))throw new Error('invalid recovery identity')
  text(input.requestId,'requestId',256);text(input.recoveryDirectory,'recoveryDirectory')
  const result={protocol:1,type:'recovery',requestId:input.requestId,runId:input.runId,status:'unknown',receipt:null,lifecycle:null,reason:'No terminal receipt; do not replay the original invocation.'}
  try {
    const id=readRecord(input.recoveryDirectory,'identity.json')
    if(id.protocol!==1||id.requestId!==input.requestId||id.runId!==input.runId)throw new Error('recovery identity mismatch')
    try {result.lifecycle=readRecord(input.recoveryDirectory,'lifecycle.json')}catch(error){if(error.code!=='ENOENT')throw error}
    if(result.lifecycle&&(result.lifecycle.requestId!==input.requestId||result.lifecycle.runId!==input.runId))throw new Error('lifecycle identity mismatch')
    try {result.receipt=readRecord(input.recoveryDirectory,'receipt.json')}catch(error){if(error.code!=='ENOENT')throw error}
    if(result.receipt) {
      if(result.receipt.type!=='result'||result.receipt.requestId!==input.requestId||result.receipt.runId!==input.runId)throw new Error('receipt identity mismatch')
      result.status='terminal';result.reason='Read the original receipt without starting or resuming a provider.'
    } else if(result.lifecycle?.dispatch==='not-dispatched') {result.status='pending';result.reason='Preparation recorded, but no terminal receipt. This read does not authorize replay.'}
    return result
  } catch(error) { return {...result,status:'unknown',receipt:null,lifecycle:null,reason:'Recovery could not verify original evidence: '+error.message} }
}
