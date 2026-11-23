'use strict';
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const {execFileSync,spawnSync}=require('node:child_process');
const {verifyCandidate}=require('./release-candidate.js');
const directory=path.resolve(process.env.FLYWHEEL_NPM_PACKAGES_DIR || 'dist-npm');
const candidate=verifyCandidate(directory,{allowDirty:true});
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'flywheel-packed-host-'));
try {
  execFileSync('tar',['-xzf',path.join(directory,candidate.packages[0].filename),'-C',temp]);
  const installed=path.join(temp,'package'),peers=process.env.FLYWHEEL_TEST_DSH_HOST_DIR ? path.resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR,'node_modules') : path.resolve(__dirname,'../plugins/dsh/node_modules');
  // Only the test harness supplies host peers. The published tarball retains
  // its bundled adapter; nothing here is repacked or installed in a user profile.
  for(const name of fs.readdirSync(peers)) {
    if(name.startsWith('.'))continue;
    const target=path.join(installed,'node_modules',name);
    if(!fs.existsSync(target))fs.symlinkSync(path.join(peers,name),target,'dir');
  }
  const tests=['console-read','client-host','host-association','tool-runtime','acp-integration'].map(name=>'test/'+name+'.test.js');
  const result=spawnSync(process.execPath,['--test','--test-concurrency=1',...tests],{
    cwd:path.resolve(__dirname,'../plugins/dsh'),stdio:'inherit',timeout:180000,
    env:{...process.env,FLYWHEEL_TEST_DSH_PACKAGE_DIR:installed},
  });
  if(result.error||result.signal||result.status!==0)throw new Error('Packed public host/client check failed');
} finally {fs.rmSync(temp,{recursive:true,force:true});}
