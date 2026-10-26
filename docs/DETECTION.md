# 检测结果的含义

## 服务与地区

每个服务独立请求，地区优先取自服务业务响应。地区后的 `*` 表示该信息来自服务域名的出口 trace；地区字段缺失时显示未知。详情中记录判定规则、HTTP 状态、地区来源和目标 URL。

| 结果 | 含义 |
|---|---|
| 解锁(地区) | 该服务的网络与地区探测通过 |
| 仅APP / 仅网页 | OpenAI 的对应入口探测通过 |
| CF验证(地区*) | 请求到达 Cloudflare 验证页，完成验证后可继续确认业务访问 |
| 失败·地区 | 服务明确拒绝该地区访问 |
| 失败·超时 | 请求超过等待时限 |
| 送中(CN) | 服务或出口响应给出 CN，原始访问结论一并保留 |

Apple TV+ 检测网页入口，Steam 检测商店入口，Wikipedia 查询本地封禁信息。OpenAI、Gemini、Claude 和 Google 分别根据各自响应作出判断。

结果描述本次请求取得的网络和地区证据。登录、付费播放和模型调用对应各自的业务流程；CF 验证对应访问验证页这一步。

## 多来源 IP 画像

### 网络类型

同一 IP 的每个有效来源计一票。显示票数最多的类型，按最保守类型评估；类型冲突时标黄，并分列展示住宅、机房等计数。来源身份、目标 IP 和查询状态通过核对后才纳入统计。

普通出口和 AI 服务出口分别查询。同一轮中，重复 IP 可复用 10 分钟内的成功结果。

### 代理识别

proxy、VPN、Tor、anonymous 等明确阳性结果计为命中；明确的代理阴性结果计为阴性。缺失字段、查询失败以及只返回 Tor=false 等局部结果归入待确认。机房属性单独列在网络类型中。

### 评分与滥用记录

各来源分别展示分值、等级、方向和查询时间。0 分保持中性色，其他来源的风险、机房、代理和匿名标签各自保留。缺失值显示未知。

AbuseIPDB 的报告数、报告人数和时间按该来源的记录展示。每个平台的滥用证据分别列出。

## 数据来源

| 来源 | 接口及主要字段 | 接入方式 |
|---|---|---|
| IPPure | my.ippure.com/v1/info：出口、ASN、fraudScore、isResidential | 公开自查 |
| ProxyCheck | proxycheck.io/v3：代理、VPN、Tor、机房、风险 | 匿名查询；密钥扩展配额 |
| NCGY | ip.nc.gy/json：OpenProxyDB 标签 | 公开查询，间隔限速 |
| ipapi.is | api.ipapi.is：数据中心、代理、VPN、滥用与网络范围评分 | API Key |
| Scamalytics | api11 / api12.scamalytics.com：fraud score、风险和上游标签 | 用户名、Key、US/EU |
| AbuseIPDB | api.abuseipdb.com/api/v2/check：过去 90 天记录和置信分 | API Key |
| IPQualityScore | ipqualityscore.com/api/json/ip：fraud_score、代理和近期滥用 | API Key |

API 失败、限流和认证失败分别记录。收到限流或认证失败后，本轮暂停该来源。ProxyCheck 另设本地日预算，实际额度以服务方响应为准。

## DNS 与 WebRTC

检查按钮打开浏览器环境检测页面，测量当前浏览器的出口、DNS 和 WebRTC 路径。这部分结果对应当前浏览器与系统路由。
