// SPDX-License-Identifier: AGPL-3.0-only
const {app,BrowserWindow,ipcMain,dialog,shell,Menu,safeStorage,session}=require('electron');
const path=require('node:path'),fs=require('node:fs/promises'),os=require('node:os');
const {pathToFileURL}=require('node:url');
const {Controller}=require('./controller.cjs');
const root=path.resolve(__dirname,'..');
let win,settingsWin,controller;
const smoke=process.argv.includes('--smoke-test');
if(smoke)app.setPath('userData',path.join(os.tmpdir(),'node-test-smoke-'+process.pid));
else app.setPath('userData',path.join(app.getPath('appData'),'NodeTestPublic'));
const mainURL=pathToFileURL(path.join(root,'static/index.html')).href;
const settingsURL=pathToFileURL(path.join(root,'desktop/settings.html')).href;
const keyIDs=['ProxyCheck','ipapi.is','scam_user','Scamalytics','scam_region','AbuseIPDB','IPQS'];
function emit(data){if(win&&!win.isDestroyed())win.webContents.send('node-test:event',data);}
async function readKeys(){try{return JSON.parse((await safeStorage.decryptStringAsync(await fs.readFile(path.join(app.getPath('userData'),'api-keys.enc')))).result);}catch(e){if(e.code==='ENOENT')return {};throw Error('无法解密数据源密钥，请检查系统账户权限');}}
function lockWindow(w){w.webContents.setWindowOpenHandler(()=>({action:'deny'}));w.webContents.on('will-navigate',e=>e.preventDefault());w.webContents.on('will-attach-webview',e=>e.preventDefault());}
function senderOK(event,w,url){return w&&!w.isDestroyed()&&event.sender===w.webContents&&event.senderFrame===w.webContents.mainFrame&&event.senderFrame.url===url;}
async function showSettings(){
 if(controller.busy)return controller.message('本轮测试完成后可修改数据源设置。');
 if(settingsWin){settingsWin.focus();return;}
 const keys=await readKeys();
 settingsWin=new BrowserWindow({parent:win,modal:true,width:700,height:620,resizable:false,title:'IP 数据源设置',webPreferences:{preload:path.join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true}});
 lockWindow(settingsWin);settingsWin.on('closed',()=>settingsWin=null);
 settingsWin.webContents.once('did-finish-load',()=>settingsWin?.webContents.send('node-test:event',{type:'settings',keys}));
 await settingsWin.loadFile(path.join(__dirname,'settings.html'));
}
async function loadFile(){if(controller.busy||controller.importing)return;const result=await dialog.showOpenDialog(win,{title:'导入订阅文件',properties:['openFile'],filters:[{name:'订阅文件',extensions:['yaml','yml','txt','conf','json']},{name:'所有文件',extensions:['*']}]});if(result.canceled)return;const file=result.filePaths[0];if((await fs.stat(file)).size>5_000_000)throw Error('订阅文件超过 5 MB');await controller.parse(await fs.readFile(file,'utf8'),path.basename(file));}
async function loadURL(text){
 if(controller.busy||controller.importing)return;
 const url=new URL(text);if(!['https:','http:'].includes(url.protocol))throw Error('订阅链接无效');
 controller.importing=true;controller.message('正在读取订阅…');
 let textContent;
 try{
  const response=await fetch(url,{headers:{'User-Agent':'clash.meta'},signal:AbortSignal.timeout(45000),redirect:'follow'});
  if(!response.ok)throw Error();
  const reader=response.body.getReader();let size=0;const chunks=[];
  try{while(true){const {value,done}=await reader.read();if(done)break;size+=value.length;if(size>5_000_000)throw Error();chunks.push(value);}}finally{await reader.cancel();}
  textContent=Buffer.concat(chunks).toString('utf8');
 }catch{throw Error('订阅读取失败。请检查链接和网络，或导入文件（最大 5 MB）。');}
 finally{controller.importing=false;}
 await controller.parse(textContent,url.hostname);
}
async function save(b){
 if(!['png','csv','json','html'].includes(b.format)||typeof b.content!=='string'||Buffer.byteLength(b.content)>100_000_000)throw Error('导出内容无效');
 const name=String(b.name||'节点测评').replace(/[<>:"/\\|?*\x00-\x1f]/g,'_').slice(0,180);
 const choice=await dialog.showSaveDialog(win,{defaultPath:name+'.'+b.format,filters:[{name:b.format.toUpperCase(),extensions:[b.format]}]});if(choice.canceled)return;
 await fs.writeFile(choice.filePath,Buffer.from(b.content,b.format==='png'?'base64':'utf8'));controller.message('已保存：'+path.basename(choice.filePath));
}
ipcMain.on('node-test:command',async(event,b)=>{
 try{
  if(!b||typeof b!=='object')return;
  if(senderOK(event,settingsWin,settingsURL)){
   if(b.cmd==='closeSettings'){settingsWin.close();return;}
   if(b.cmd==='saveSettings'){
    if(controller.busy)throw Error('测试中暂时无法修改设置');
    if(!await safeStorage.isAsyncEncryptionAvailable())throw Error('系统加密存储暂不可用');
    const keys={};for(const k of keyIDs)if(typeof b.keys?.[k]==='string'&&b.keys[k].trim())keys[k]=b.keys[k].trim().slice(0,4096);
    keys.scam_region=(keys.scam_region||'US').toUpperCase();if(!['US','EU'].includes(keys.scam_region))throw Error('Scamalytics 节点请填写 US 或 EU');
    await controller.atomic(path.join(app.getPath('userData'),'api-keys.enc'),await safeStorage.encryptStringAsync(JSON.stringify(keys)));settingsWin.close();controller.message('数据源密钥已使用系统加密存储保存。');
   }return;
  }
  if(!senderOK(event,win,mainURL))return;
  switch(b.cmd){
   case 'ready': {const items=Object.entries(os.networkInterfaces()).filter(([,a])=>a.some(x=>x.family==='IPv4'&&!x.internal)).map(([n])=>n);emit({type:'interfaces',items,preferred:''});await controller.history();break;}
   case 'riskSettings':await showSettings();break;
   case 'loadFile':await loadFile();break;
   case 'loadURL':if(typeof b.url==='string')await loadURL(b.url);break;
   case 'start':{
    const options=b.options||{};if(options.interface&&!Object.hasOwn(os.networkInterfaces(),options.interface))throw Error('出站网卡已失效，请重新选择');
    await controller.start(options);break;
   }
   case 'cancel':controller.stop();break;
   case 'save':await save(b);break;
   case 'history':await controller.loadHistory(b.id);break;
   case 'reference':{const u=new URL(b.url);if(u.protocol==='https:'&&['ipleak.net','peanutdl.com','ipinfo.cv','ip.net.coffee','ippure.com','proxycheck.io','ip.nc.gy','support.claude.com'].includes(u.hostname))await shell.openExternal(u.href);break;}
  }
 }catch(e){controller?.message(b?.cmd==='loadURL'?'订阅读取失败，请检查链接和网络，或导入本地文件。':e.message||'操作失败',true);}
});
app.whenReady().then(async()=>{
 session.defaultSession.setPermissionRequestHandler((_w,_p,cb)=>cb(false));session.defaultSession.setPermissionCheckHandler(()=>false);
 win=new BrowserWindow({width:1450,height:870,minWidth:960,minHeight:640,show:!smoke,title:'节点测评',webPreferences:{preload:path.join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true}});
 lockWindow(win);
 const worker=app.isPackaged?path.join(process.resourcesPath,'node-worker.exe'):path.join(root,'bin',process.platform==='win32'?'node-worker.exe':'node-worker');
 controller=new Controller({worker,support:app.getPath('userData'),emit,readKeys});await controller.init();
 Menu.setApplicationMenu(Menu.buildFromTemplate([{label:'文件',submenu:[{label:'导入订阅文件',click:()=>loadFile().catch(e=>controller.message(e.message,true))},{label:'IP 数据源设置',click:()=>showSettings().catch(e=>controller.message(e.message,true))},{label:'结果文件夹',click:()=>shell.openPath(path.join(app.getPath('userData'),'reports'))},{type:'separator'},{role:'quit'}]},{label:'编辑',submenu:[{role:'undo'},{role:'redo'},{type:'separator'},{role:'cut'},{role:'copy'},{role:'paste'},{role:'selectAll'}]},{label:'帮助',submenu:[{label:'源码与许可证',click:()=>shell.openExternal('https://github.com/WWangZQ/node-test')},{label:'关于',click:()=>dialog.showMessageBox(win,{message:'节点测评 1.3.1',detail:'本机全订阅测速与服务检测\nAGPL-3.0；上游组件保留各自许可证。'})}]}]));
 await win.loadFile(path.join(root,'static/index.html'));
 if(smoke){try{await require('./smoke.cjs')({win,controller,root,app});app.exit(0);}catch(e){console.error('Smoke test failed:',e.message);app.exit(1);}}
}).catch(e=>{console.error(e.message);app.exit(1);});
app.on('before-quit',()=>controller?.stop());app.on('window-all-closed',()=>app.quit());
