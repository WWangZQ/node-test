const assert=require('node:assert/strict');
const R=require('../static/risk-model.js');
const ip='203.0.113.1';
const src=(source,data)=>({source,ip,status:'ok',...data});
const data={IPPure:src('IPPure',{network_type:'住宅',flags:{isResidential:true}}),ProxyCheck:src('ProxyCheck',{network_type:'Residential',score:0,flags:{proxy:false}}),NCGY:src('NCGY',{flags:{is_hosting:true,is_proxy:true,is_anonymous:true}}),'ipapi.is':src('ipapi.is',{flags:{is_datacenter:true,is_proxy:true,is_abuser:true}}),Scamalytics:src('Scamalytics',{network_type:'Residential'})};
let a=R.summarize(data,ip);
assert.equal(a.majority,'住宅');assert.equal(a.worst,'机房');assert.deepEqual(a.counts,{'住宅':3,'机房':2});assert.equal(a.conflict,true);assert.equal(a.warning,true);assert.deepEqual(a.proxy.yes,['NCGY','ipapi.is']);assert.deepEqual(a.proxy.no,['ProxyCheck']);assert.equal(a.abuse.yes.length,1);
assert.equal(R.summarize(data,'203.0.113.2').coverage,0,'different IP must not inherit votes');
assert.equal(R.summarize({},ip).proxy.no.length,0,'missing source is not negative');
const missing=src('ipapi.is',{flags:{is_datacenter:false,is_proxy:null},network_type:''});
assert.equal(R.network(missing),'未识别','not hosting does not imply residential');assert.equal(R.proxy(missing),null);
const tie=R.summarize({IPPure:data.IPPure,NCGY:data.NCGY},ip);assert.equal(tie.majority,'机房(1) / 住宅(1)');assert.equal(tie.worst,'机房');
const duplicate=R.summarize({NCGY:data.NCGY,Scamalytics:{...data.NCGY}},ip);assert.equal(duplicate.proxy.yes.length,1,'source aliases do not duplicate votes');
assert.equal(R.summarize({IPPure:src('IPPure',{flags:{isResidential:true}})},ip).coverage,1);
assert.equal(R.network(src('IPPure',{flags:{isResidential:false}})),'非住宅');
assert.equal(R.proxy(src('NCGY',{flags:{is_hosting:true}})),null,'hosting alone is not proxy evidence');
console.log('IP aggregation assertions passed');

assert.equal(R.proxy(src('AbuseIPDB',{flags:{isTor:false}})),null,'Tor negative does not establish proxy negative');

const hk=R.summarize({IPPure:src('IPPure',{network_type:'住宅',flags:{isResidential:true}}),ProxyCheck:src('ProxyCheck',{network_type:'Wireless'}),AbuseIPDB:src('AbuseIPDB',{network_type:'Fixed Line ISP'})},ip);
assert.equal(hk.majority,'住宅');assert.equal(hk.worst,'住宅');assert.equal(hk.conflict,false);assert.deepEqual(hk.accessCounts,{'Wireless':1,'Fixed ISP':1});
assert.equal(R.access(src('ProxyCheck',{network_type:'Wireless'})),'Wireless');
assert.equal(R.access(src('ipapi.is',{network_type:'isp'})),'ISP');
assert.equal(R.summarize({AbuseIPDB:src('AbuseIPDB',{network_type:'Fixed Line ISP'})},ip).majority,'用途待定');
