'use strict';
// Shared by the live table, export and tests. Counts represent providers, not independent databases.
const RiskModel=(()=>{
 const providers=['IPPure','ProxyCheck','NCGY','ipapi.is','Scamalytics','AbuseIPDB','IPQS'];
 const states={config:'配置有误',not_configured:'未配置',limited:'限流',auth:'鉴权失败',skipped:'额度用完',error:'查询失败',partial:'字段不全',unknown:'未判定',denied:'拒绝访问'};
 const aliases={proxy:['proxy','is_proxy'],vpn:['vpn','is_vpn','active_vpn'],tor:['tor','is_tor','isTor','active_tor'],hosting:['hosting','is_hosting','is_datacenter'],anonymous:['anonymous','is_anonymous'],resproxy:['is_resproxy'],relay:['is_apple_icloud_private_relay']};
 function flag(s,k){const a=(aliases[k]||[k]).map(x=>s.flags?.[x]).filter(v=>typeof v==='boolean');if(a.includes(true))return true;if(a.length)return false;return null}
 function network(s){if(flag(s,'hosting')===true)return '机房';if(s.flags?.isResidential===true)return '住宅';const t=(s.network_type||'').toLowerCase().trim();if(/hosting|datacenter|data cent|content delivery|\bdch\b|机房/.test(t))return '机房';if(/residential|住宅|家宽/.test(t)&&!/^非/.test(t))return '住宅';if(/business|commercial|商用|\bcom\b/.test(t))return '商用';if(/education|university|school|\bedu\b/.test(t))return '教育';if(s.flags?.isResidential===false||t==='非住宅')return '非住宅';return '未识别'}
 function access(s){const t=(s.network_type||'').toLowerCase().trim();if(s.flags?.is_mobile===true||/mobile|cellular|\bmob\b/.test(t))return 'Cellular';if(t==='wireless')return 'Wireless';if(t==='fixed line isp')return 'Fixed ISP';if(t==='isp')return 'ISP';if(/satellite/.test(t))return 'Satellite';return ''}
 function proxy(s){const a=['proxy','vpn','tor','anonymous','resproxy','relay'].map(k=>flag(s,k));return a.includes(true)?true:flag(s,'proxy')===false?false:null}
 function abuse(s){if(typeof s.abuse?.listed==='boolean')return s.abuse.listed;const a=['is_abuser','recent_abuse','compromised'].map(k=>s.flags?.[k]).filter(v=>typeof v==='boolean');return a.includes(true)?true:a.length?false:null}
 function samples(data,ip){return providers.map(name=>{let s=data?.[name];if(!s)return {source:name,status:'not_tested'};if(s.source!==name||s.ip!==ip)return {source:name,status:'mismatch',detail:'来源或目标 IP 不匹配，未计入汇总'};return s})}
 function summarize(data,ip){const all=samples(data,ip),ok=all.filter(s=>s.status==='ok'),counts={},votes={};for(const s of ok){const n=network(s);if(n!=='未识别'){counts[n]=(counts[n]||0)+1;(votes[n]??=[]).push(s.source)}}
 const accessCounts={},accessVotes={};for(const s of ok){const n=access(s);if(n){accessCounts[n]=(accessCounts[n]||0)+1;(accessVotes[n]??=[]).push(s.source)}}
 const rank={'机房':6,'非住宅':5,'商用':4,'教育':2,'住宅':0};const sorted=Object.entries(counts).sort((a,b)=>b[1]-a[1]||rank[b[0]]-rank[a[0]]);const winners=sorted.filter(x=>x[1]===sorted[0]?.[1]).map(x=>x[0]);const worst=Object.keys(counts).sort((a,b)=>rank[b]-rank[a])[0]||'未识别';
 const counted=fn=>{const yes=ok.filter(s=>fn(s)===true).map(s=>s.source),no=ok.filter(s=>fn(s)===false).map(s=>s.source);return {yes,no,unknown:providers.length-yes.length-no.length}};
 const p=counted(proxy),a=counted(abuse),tags={};for(const k of ['vpn','tor','anonymous','resproxy'])tags[k]=counted(s=>flag(s,k));
 const high=ok.filter(s=>s.source==='Scamalytics'&&['high','very high','very_high'].includes(s.level?.toLowerCase()));
 return {sources:all,coverage:ok.length,total:providers.length,counts,votes,accessCounts,accessVotes,majority:winners.length>1?winners.map(n=>n+'('+counts[n]+')').join(' / '):winners[0]||'用途待定',worst,conflict:sorted.length>1,proxy:p,abuse:a,tags,high:high.map(s=>s.source),warning:sorted.length>1||worst==='机房'||p.yes.length>0||a.yes.length>0||high.length>0};
 }
 function state(s){return states[s?.status]||({not_tested:'未测',mismatch:'IP不匹配',ok:'已查询'}[s?.status])||'未测'}
 return {providers,states,network,access,proxy,abuse,flag,summarize,state};
})();
if(typeof module!=='undefined')module.exports=RiskModel;
