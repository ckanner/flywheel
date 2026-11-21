import { closeSync, constants, lstatSync, mkdirSync, openSync, readFileSync, realpathSync, rmdirSync, unlinkSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { hash } from './input.js'

/** Native custom-agent allowlist, guarded again against the CLI's init.tools. */
export function prepareEvidenceAgent(cwd,runId) {
  const created=[]
  for(const path of [join(cwd,'.agents'),join(cwd,'.agents','agents')]){
    try{mkdirSync(path,{mode:0o700});created.push(path)}catch(error){if(error.code!=='EEXIST')throw error}
    const stat=lstatSync(path)
    if(!stat.isDirectory()||stat.isSymbolicLink()||realpathSync(path)!==path)throw new Error('evidence-only agent directory must be a physical workspace directory')
  }
  const name='dsh-evidence-'+runId,path=join(cwd,'.agents','agents',name+'.md')
  const content='---\nname: '+name+'\ndescription: Analyze only the evidence included in the user message.\nmainAgent: true\nsubagent: false\nmodel: inherit\ntools: []\nmcpServers: []\nskills: []\nplugins: []\ncommandExecutionPolicy: "off"\n---\nAnalyze the supplied evidence. Identify gaps without collecting additional evidence.\n'
  const fd=openSync(path,constants.O_WRONLY|constants.O_CREAT|constants.O_EXCL|constants.O_NOFOLLOW,0o600)
  try{writeFileSync(fd,content)}finally{closeSync(fd)}
  const inode=lstatSync(path).ino
  const verify=()=>{
    const stat=lstatSync(path)
    if(stat.isSymbolicLink()||stat.ino!==inode||readFileSync(path,'utf8')!==content)throw new Error('managed evidence-only agent changed during invocation')
  }
  return {name,path,digest:hash(content),ownerRunId:runId,verify,dispose(){
    verify();unlinkSync(path)
    for(const directory of created.reverse())try{rmdirSync(directory)}catch(error){if(!['ENOTEMPTY','EEXIST'].includes(error.code))throw error}
  }}
}
