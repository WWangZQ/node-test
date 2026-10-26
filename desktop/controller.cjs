// SPDX-License-Identifier: AGPL-3.0-only
const {randomUUID} = require('node:crypto');
const path = require('node:path');
const fs = require('node:fs/promises');
const {spawn} = require('node:child_process');
const date = () => new Date().toISOString();
const validID = id => typeof id === 'string' && /^[a-f0-9-]{36}$/i.test(id);
class Controller {
  constructor({worker, support, emit, readKeys = async () => ({}), execute}) {
    Object.assign(this, {worker, support, emit, readKeys});
    this.nodes = []; this.busy = false; this.importing = false;
    this.execute = execute || this.runWorker.bind(this);
  }
  message(text, error = false) { this.emit({type:'message', text, error}); }
  async init() { await fs.mkdir(path.join(this.support,'reports'),{recursive:true}); }
  runWorker(input) {
    return new Promise((resolve, reject) => {
      const p = spawn(this.worker, [], {cwd:this.support, windowsHide:true, stdio:['pipe','pipe','ignore']});
      this.child = p; let output = ''; let finished = false;
      const finish = (error, result) => { if(finished)return; finished=true; clearTimeout(timer); if(this.child===p)this.child=null; error?reject(error):resolve(result); };
      // Worker has its own request deadlines. This is a final guard against a stuck process.
      const timer = setTimeout(()=>{p.kill();finish(new Error('检测进程超时'));}, 20*60*1000);
      p.on('error',()=>finish(new Error('检测引擎无法启动')));
      p.stdin.on('error',()=>{});
      p.stdout.on('data',b=>{output+=b.toString();if(output.length>64*1024*1024){p.kill();finish(new Error('检测输出过大'));}});
      p.on('close',()=>{try{const line=output.split(/\r?\n/).filter(s=>s.startsWith('RESULT:')).at(-1);if(!line)throw Error();finish(null,JSON.parse(line.slice(7)));}catch{finish(new Error('检测进程退出，已跳过此节点'));}});
      p.stdin.end(JSON.stringify(input));
    });
  }
  async parse(text, name='订阅') {
    if(this.busy||this.importing)return;
    this.importing=true;
    try {
      if(Buffer.byteLength(text)>5_000_000)throw Error('订阅超过 5 MB');
      const result=await this.execute({action:'parse',content:text});
      if(!Array.isArray(result.nodes)||!result.nodes.length)throw Error(result.error||'订阅解析失败');
      this.nodes=result.nodes;this.name=String(name).slice(0,100);
      this.report={id:randomUUID(),name:this.name,status:'ready',demo:false,created:date(),rows:this.rows()};
      this.emit({type:'report',report:this.report});this.emit({type:'source',count:this.nodes.length});
    } finally {this.importing=false;}
  }
  rows(){return this.nodes.map((n,i)=>({name:String(n.name||`节点 ${i+1}`).slice(0,200),type:String(n.type||'unknown'),status:'pending',services:{},samples:[]}));}
  async atomic(file, data) {const temp=file+'.tmp';await fs.writeFile(temp,data,{mode:0o600});await fs.rename(temp,file);}
  async persist(){await this.atomic(path.join(this.support,'reports',this.report.id+'.json'),JSON.stringify(this.report,null,2));}
  async history(){
    const items=[];
    for(const file of await fs.readdir(path.join(this.support,'reports'))){
      if(!file.endsWith('.json')||!validID(file.slice(0,-5)))continue;
      try{const p=path.join(this.support,'reports',file),r=JSON.parse(await fs.readFile(p,'utf8'));
        if(!this.busy&&r.status==='running'){r.status='interrupted';for(const row of r.rows||[])if(['pending','running'].includes(row.status))row.status='interrupted';await this.atomic(p,JSON.stringify(r,null,2));}
        items.push({id:r.id,name:r.name,created:r.created});
      }catch{/* Ignore incomplete or invalid reports. */}
    }
    items.sort((a,b)=>String(b.created).localeCompare(String(a.created)));this.emit({type:'history',items});
  }
  async loadHistory(id){if(this.busy||!validID(id))return;const r=JSON.parse(await fs.readFile(path.join(this.support,'reports',id+'.json'),'utf8'));this.report=r;this.emit({type:'report',report:r});}
  stop(){this.cancelled=true;this.child?.kill();}
  async start(options={}) {
    if(this.busy||this.importing||!this.nodes.length)return;
    this.busy=true;this.cancelled=false;
    try {
      const risk=options.ip_risk!==false,keys=risk?await this.readKeys():{};
      if(this.cancelled)return;
      const clamp=(n,min,max,fallback)=>Math.max(min,Math.min(max,Number.isInteger(n)?n:fallback));
      const settings={mode:['both','speed','unlock'].includes(options.mode)?options.mode:'both',seconds:clamp(options.seconds,2,15,8),megabytes:clamp(options.megabytes,1,256,64),timeout:8,ip_info:true,interface:String(options.interface||''),platforms:[...new Set(Array.isArray(options.platforms)?options.platforms:[])].filter(x=>typeof x==='string'&&x.length<80).slice(0,60),ip_risk:risk};
      const r=this.report={id:randomUUID(),name:this.name,status:'running',demo:false,created:date(),rows:this.rows(),settings,units:{speed:'MB/s',latency:'ms',bytes:'bytes'},probe_version:'1.3',engine:'mihomo 1.19.10 / Node Test 1.3.1'};
      const usageFile=path.join(this.support,'quota.json');let usage={day:date().slice(0,10),used:0};
      try{const old=JSON.parse(await fs.readFile(usageFile,'utf8'));if(old.day===usage.day&&Number.isFinite(old.used))usage.used=Math.max(0,old.used);}catch{}
      const budget=keys.ProxyCheck?1000:100;let remaining=Math.max(0,budget-usage.used),cache={},disabled={};
      await this.persist();this.emit({type:'report',report:r});
      for(let i=0;i<this.nodes.length;i++){
        if(this.cancelled)break;r.rows[i].status='running';this.emit({type:'report',report:r});
        try{
          const result=await this.execute({...settings,action:'test',node:this.nodes[i],risk_keys:keys,risk_cache:cache,risk_disabled:disabled,proxycheck_remaining:remaining});
          if(this.cancelled){r.rows[i].status='cancelled';break;}
          const count=Math.max(0,Number(result.proxycheck_requests)||0);remaining=Math.max(0,remaining-count);usage.used+=count;
          if(result.proxycheck_limited){remaining=0;usage.used=budget;}
          await this.atomic(usageFile,JSON.stringify(usage));
          Object.assign(cache,result.risk_updates||{});Object.assign(disabled,result.risk_stopped||{});
          const {risk_updates,risk_stopped,...safe}=result;Object.assign(r.rows[i],safe);
        }catch{r.rows[i].status=this.cancelled?'cancelled':'failed';r.rows[i].error=this.cancelled?'用户停止测试':'检测进程退出，已跳过此节点';}
        await this.persist();this.emit({type:'report',report:r});
      }
      if(this.cancelled)for(const row of r.rows)if(['pending','running'].includes(row.status))row.status='cancelled';
      r.status=this.cancelled?'cancelled':'complete';r.finished=date();await this.persist();this.emit({type:'report',report:r});this.message(this.cancelled?'已停止，已完成的结果可以导出。':'测试完成。结果已保存在本机。');
    } catch(error) {
      if(this.report?.status==='running'){this.report.status='interrupted';for(const row of this.report.rows)if(['running','pending'].includes(row.status))row.status='interrupted';await this.persist().catch(()=>{});this.emit({type:'report',report:this.report});}
      throw error;
    } finally {this.busy=false;await this.history();}
  }
}
module.exports={Controller,validID};
