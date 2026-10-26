# 检测结果的含义

## 服务与地区

每个服务独立请求，地区优先采用服务业务响应。仅取得服务域名出口 trace 时，地区后附 `*`，并在详情说明来源。服务没有返回地区则保留未知，避免套用节点名称或其他服务的出口。

- 解锁(地区)：该服务专用网络 / 地区探测通过。
- 仅APP / 仅网页：OpenAI 两个入口的结果不同。
- CF验证(地区*)：到达 Cloudflare 验证页，仍需完成验证，业务可用性待核实。
- 失败·地区 / 失败·超时：分别对应明确地区拒绝与连接超时。
- 送中(CN)：服务或对应出口证据给出 CN，保留原始失败或可用结论，不能据此推断服务器物理位置。
- Apple TV+：验证网页入口；Steam：验证商店；Wikipedia：只读检查本地封禁信息。

OpenAI、Gemini、Claude、Google 的判定分别读取各自响应。不会用 Claude 官方支持国家名单代替节点实测，也不会把一般首页 HTTP 200 作为所有服务完整解锁的证据。详情保留具体规则、HTTP 状态、地区来源与目标 URL。

## 多来源 IP 画像

同一个 IP 的不同来源各计一票。未知值、没有字段、查询失败不参与正负票。来源身份和目标 IP 必须匹配。普通出口与 AI 服务出口分开查询，重复 IP 在一轮中复用最多 10 分钟的成功结果。

网络多数类型用于显示；机房等较保守的观测用于评估。类型冲突时标黄，单列展示各类型计数。机房本身不会伪造代理命中；明确的 proxy / VPN / Tor / anonymous 等阳性参与代理计数，仅 Tor=false 不足以判断没有代理。

各家评分保留自己的分值、等级、方向和查询时间，避免将不同模型的分数相加或平均。来源缺失时显示未知。0 分保持中性，无法消除其他来源给出的高风险、机房、代理或匿名证据。AbuseIPDB 的报告数和报告人数只在该来源范围内计数，跨平台记录不重复相加。

## 来源

| 来源 | 接口及主要字段 | 要求 |
|---|---|---|
| IPPure | my.ippure.com/v1/info，实际出口、ASN、fraudScore、isResidential | 公开自查 |
| ProxyCheck | proxycheck.io/v3，代理、VPN、Tor、机房、风险 | 可匿名，密钥扩展配额 |
| NCGY | ip.nc.gy/json，OpenProxyDB 标签 | 公开查询、间隔限速 |
| ipapi.is | api.ipapi.is，数据中心、代理、VPN、滥用与网络范围评分 | API Key |
| Scamalytics | api11 / api12.scamalytics.com，fraud score、风险和上游标签 | 用户名、Key、US/EU |
| AbuseIPDB | api.abuseipdb.com/api/v2/check，过去 90 天记录和置信分 | API Key，只查询 |
| IPQualityScore | ipqualityscore.com/api/json/ip，fraud_score、代理和近期滥用 | API Key、可选 |

API 失败、限流和认证失败单独记录；限流或认证失败后，本轮停止继续请求该来源。ProxyCheck 另设本地日预算。共享出口、同账户其他设备和套餐差异可能让服务方额度提前耗尽。

DNS / WebRTC 检查按钮打开浏览器环境检测页面，测量当前浏览器和系统路由。它们无法代表列表中每个节点的 DNS / WebRTC 情况。
