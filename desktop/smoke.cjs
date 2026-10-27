// End-to-end fixture validation through the packaged worker and real renderer.
const assert=require('node:assert/strict');
const fs=require('node:fs/promises'),path=require('node:path');
module.exports=async({win,controller,root,app})=>{
 const title=await win.webContents.executeJavaScript('document.querySelector("h1").textContent');assert.equal(title,'节点测评');
 assert.equal(await win.webContents.executeJavaScript("document.querySelector('#check-updates').textContent"),'检查更新');
 const {safeStorage}=require('electron');
 assert.equal(await safeStorage.isAsyncEncryptionAvailable(),true);
 const secret='fixture-key-never-saved';assert.equal((await safeStorage.decryptStringAsync(await safeStorage.encryptStringAsync(secret))).result,secret);
 const sandbox=await win.webContents.executeJavaScript('typeof require');assert.equal(sandbox,'undefined');
 const yaml='proxies:\n- {name: Dead A, type: socks5, server: 127.0.0.1, port: 9}\n- {name: Dead B, type: socks5, server: 127.0.0.1, port: 9}\n';
 await controller.parse(yaml,'Local fixture');assert.equal(controller.nodes.length,2);
 await controller.start({mode:'speed',seconds:2,megabytes:1,platforms:[],ip_risk:false});
 assert.equal(controller.report.status,'complete');assert.ok(controller.report.rows.every(r=>r.status==='failed'));assert.ok(controller.report.rows.every(r=>r.average_MBps==null));
 const result=await win.webContents.executeJavaScript(`(()=>({csv:exportCSV(),html:exportHTML(),json:exportReport(),rows:document.querySelectorAll('#results tbody tr').length}))()`);
 assert.equal(result.rows,2);assert.match(result.csv,/Dead A/);assert.match(result.html,/Dead B/);assert.equal(result.json.rows.length,2);
 const png=await win.webContents.executeJavaScript('exportPNG()');assert.ok(Buffer.from(png,'base64').subarray(1,4).equals(Buffer.from('PNG')));
 // Restore synthetic demonstration and capture the public system-font interface.
 await new Promise(resolve=>setTimeout(resolve,300));
 await win.webContents.executeJavaScript("showDemo();extendDemo();sourceCount=0;render();message('以下为合成示例，未进行真实网络测试。')");
 await new Promise(resolve=>setTimeout(resolve,500));
 assert.match(await win.webContents.executeJavaScript("document.querySelector('#badge').textContent"),/示例/);
 const screenshot=await win.webContents.capturePage();
 const output=process.env.NODE_TEST_SMOKE_OUTPUT||path.join(root,'dist');await fs.mkdir(output,{recursive:true});
 await fs.writeFile(path.join(output,'smoke-preview.png'),screenshot.toPNG());
 await fs.writeFile(path.join(output,'smoke-result.json'),JSON.stringify({platform:process.platform,arch:process.arch,version:app.getVersion(),rows:2,unreachable:'failed',exports:['png','csv','json','html'],rendererNodeIntegration:false},null,2));
 console.log('Desktop smoke test passed: two failed nodes, renderer bridge, four exports');
};
