// SPDX-License-Identifier: AGPL-3.0-only
const {execFile,spawn}=require('node:child_process');
const fs=require('node:fs/promises'),path=require('node:path'),os=require('node:os');
function runHelper(executable,args){return new Promise((resolve,reject)=>execFile(executable,args,{timeout:18*60*1000,maxBuffer:1024*1024,windowsHide:true},(error,stdout)=>{let v;try{v=JSON.parse(stdout)}catch{return reject(Error('更新服务响应异常，请重试'))}if(error||v.error)return reject(Error(v.error||'更新准备失败'));resolve(v)}))}
function createUpdater({app,dialog,win,controller,helper,run=runHelper}){
 let active=false;const file=path.join(app.getPath('userData'),'update-state.json');
 const busy=()=>controller.busy||controller.importing;
 async function check(automatic=false){
  if(active)return;if(busy()){if(!automatic)controller.message('本轮测试完成后可更新应用。');return}
  let interactive=!automatic;
  let state={};try{state=JSON.parse(await fs.readFile(file,'utf8'))}catch{}
  if(state.stagingDir&&path.dirname(state.stagingDir)===os.tmpdir()&&path.basename(state.stagingDir).startsWith('node-test-update-')){try{await fs.rm(state.stagingDir,{recursive:true,force:true});delete state.stagingDir}catch{}}
  if(automatic&&Date.now()-(state.checked||0)<86400000)return;
  active=true;
  try{
   if(!automatic)controller.message('正在检查更新…');
   const update=await run(helper,['check','--current',app.getVersion()]);state.checked=Date.now();await controller.atomic(file,JSON.stringify(state));
   if(!update.available){if(!automatic)controller.message('当前已是最新版本 '+app.getVersion());return}
   if(busy()||(automatic&&state.reminded===update.version))return;
   state.reminded=update.version;await controller.atomic(file,JSON.stringify(state));
   const choice=await dialog.showMessageBox(win,{type:'info',message:'节点测评 '+update.version+' 已发布',detail:'下载后重启应用。测试记录与设置保留。',buttons:['下载并重启','稍后'],defaultId:0,cancelId:1});
   if(choice.response!==0||busy())return;interactive=true;
   controller.message('正在下载并校验更新…');win.setProgressBar(2);
   const ready=await run(helper,['prepare','--current',app.getVersion(),'--version',update.version]);
   const plan=JSON.parse(await fs.readFile(ready.plan,'utf8'));
   // The helper supplies a verified installer in its private staging directory.
   if(path.dirname(plan.Installer)!==path.dirname(ready.plan)||!path.basename(plan.Installer).endsWith('-Setup.exe'))throw Error('更新安装包路径校验失败');
   state.stagingDir=path.dirname(ready.plan);await controller.atomic(file,JSON.stringify(state));
   const child=spawn(plan.Installer,['/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/CLOSEAPPLICATIONS','/UPDATE=1','/DIR='+path.dirname(app.getPath('exe'))],{detached:true,stdio:'ignore',windowsHide:true});
   await new Promise((resolve,reject)=>{child.once('spawn',resolve);child.once('error',reject)});child.unref();app.quit();
  }catch(e){if(interactive)controller.message(e.message,true)}finally{active=false;if(!win.isDestroyed())win.setProgressBar(-1)}
 }
 return {check,get active(){return active}};
}
module.exports={createUpdater,runHelper};
