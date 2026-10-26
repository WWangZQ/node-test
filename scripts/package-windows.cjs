const {packager}=require('@electron/packager');
const fs=require('node:fs/promises'),path=require('node:path');
(async()=>{
 const root=path.resolve(__dirname,'..');
 const stage=path.join(root,'dist','windows-stage');await fs.rm(stage,{recursive:true,force:true});await fs.mkdir(stage,{recursive:true});
 for(const file of ['desktop','static','package.json','LICENSE','LICENSES','THIRD_PARTY_NOTICES.md'])await fs.cp(path.join(root,file),path.join(stage,file),{recursive:true});
 await packager({dir:stage,out:path.join(root,'dist'),name:'NodeTest',platform:'win32',arch:'x64',electronVersion:require('../package.json').devDependencies.electron,overwrite:true,asar:true,prune:false,extraResource:[path.join(root,'bin/node-worker.exe')],appCopyright:'WWangZQ and contributors; see THIRD_PARTY_NOTICES.md'});
 console.log('Packaged dist/NodeTest-win32-x64');
})().catch(e=>{console.error(e);process.exit(1)});
