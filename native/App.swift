import Cocoa
import WebKit
import Security
import UniformTypeIdentifiers
import Darwin

// Bounded, memory-only subscription fetching. Credentials never enter URLCache or a download file.
final class SubscriptionLoader: NSObject, URLSessionDataDelegate {
    var bytes=Data()
    var valid=false
    var session:URLSession?
    let done:(String?)->Void
    init(url:URL,done:@escaping(String?)->Void){self.done=done;super.init();let config=URLSessionConfiguration.ephemeral;config.urlCache=nil;config.httpCookieStorage=nil;config.timeoutIntervalForRequest=30;config.timeoutIntervalForResource=45;session=URLSession(configuration:config,delegate:self,delegateQueue:nil);var req=URLRequest(url:url,cachePolicy:.reloadIgnoringLocalCacheData,timeoutInterval:30);req.setValue("clash.meta",forHTTPHeaderField:"User-Agent");session!.dataTask(with:req).resume()}
    func urlSession(_ session:URLSession,dataTask:URLSessionDataTask,didReceive response:URLResponse,completionHandler:@escaping(URLSession.ResponseDisposition)->Void){valid=(response as? HTTPURLResponse).map{(200...299).contains($0.statusCode)} ?? false;if response.expectedContentLength>5_000_000{valid=false};completionHandler(valid ? .allow:.cancel)}
    func urlSession(_ session:URLSession,dataTask:URLSessionDataTask,didReceive data:Data){if bytes.count+data.count>5_000_000{valid=false;dataTask.cancel()}else{bytes.append(data)}}
    func urlSession(_ session:URLSession,task:URLSessionTask,didCompleteWithError error:Error?){let result=(error==nil && valid) ? String(data:bytes,encoding:.utf8):nil;bytes.removeAll();session.finishTasksAndInvalidate();self.session=nil;done(result)}
}

// Node Test distribution is AGPL-3.0. No web server is launched; the UI is bundled app content.
final class App: NSObject, NSApplicationDelegate, WKScriptMessageHandler, WKNavigationDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var nodes: [[String: Any]] = []
    var sourceName = "订阅"
    var report: [String: Any]?
    var busy = false
    var importing = false
    var loader:SubscriptionLoader?
    var cancelled = false
    var current: Process?
    let lock = NSLock()
    let queue = DispatchQueue(label: "local-node-test.worker", qos: .userInitiated)
    lazy var support: URL = {
        let u = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("NodeTestPublic", isDirectory: true)
        try? FileManager.default.createDirectory(at: u.appendingPathComponent("reports"), withIntermediateDirectories: true)
        return u
    }()
    var worker: URL { Bundle.main.resourceURL!.appendingPathComponent("node-worker") }
    func applicationDidFinishLaunching(_ n: Notification) {
        _ = support
        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "bridge")
        web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        web.setValue(false, forKey: "drawsBackground")
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1450, height: 870), styleMask: [.titled,.closable,.miniaturizable,.resizable], backing: .buffered, defer: false)
        window.title = "节点测评"
        window.minSize = NSSize(width: 960, height: 640)
        window.contentView = web
        window.center()
        window.setFrameAutosaveName("LocalNodeTest.Main")
        makeMenu()
        let ui = Bundle.main.resourceURL!.appendingPathComponent("ui")
        web.loadFileURL(ui.appendingPathComponent("index.html"), allowingReadAccessTo: ui)
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }
    func makeMenu() {
        let menu = NSMenu(); let mainItem = NSMenuItem(); menu.addItem(mainItem)
        let appMenu = NSMenu(); mainItem.submenu = appMenu
        appMenu.addItem(withTitle: "关于节点测评", action: #selector(about), keyEquivalent: "")
        appMenu.addItem(withTitle:"IP 数据源设置…", action:#selector(riskSettings), keyEquivalent:",")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "退出节点测评", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        let fileItem = NSMenuItem(); menu.addItem(fileItem); let fileMenu = NSMenu(title:"文件");fileItem.submenu=fileMenu
        fileMenu.addItem(withTitle:"导入订阅文件…", action:#selector(openFile), keyEquivalent:"o")
        fileMenu.addItem(withTitle:"显示结果文件夹", action:#selector(showResults), keyEquivalent:"")
        let editItem = NSMenuItem();menu.addItem(editItem);let edit = NSMenu(title:"编辑");editItem.submenu=edit
        edit.addItem(withTitle:"撤销",action:Selector(("undo:")),keyEquivalent:"z")
        edit.addItem(withTitle:"剪切",action:#selector(NSText.cut(_:)),keyEquivalent:"x")
        edit.addItem(withTitle:"拷贝",action:#selector(NSText.copy(_:)),keyEquivalent:"c")
        edit.addItem(withTitle:"粘贴",action:#selector(NSText.paste(_:)),keyEquivalent:"v")
        edit.addItem(withTitle:"全选",action:#selector(NSText.selectAll(_:)),keyEquivalent:"a")
        let winItem = NSMenuItem();menu.addItem(winItem);let winMenu=NSMenu(title:"窗口");winItem.submenu=winMenu
        winMenu.addItem(withTitle:"最小化",action:#selector(NSWindow.miniaturize(_:)),keyEquivalent:"m")
        NSApp.mainMenu=menu
    }
    func readRiskKeys()->[String:String]? {
        let query:[String:Any]=[kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"io.github.WWangZQ.node-test.risk",kSecAttrAccount as String:"api-keys",kSecReturnData as String:true,kSecMatchLimit as String:kSecMatchLimitOne]
        var item:CFTypeRef?;let status=SecItemCopyMatching(query as CFDictionary,&item)
        if status==errSecItemNotFound{return [:]}
        guard status==errSecSuccess,let data=item as? Data,let keys=try? JSONSerialization.jsonObject(with:data) as? [String:String] else{return nil}
        return keys
    }
    func saveRiskKeys(_ keys:[String:String])->Bool {
        guard let data=try? JSONSerialization.data(withJSONObject:keys) else{return false}
        let query:[String:Any]=[kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"io.github.WWangZQ.node-test.risk",kSecAttrAccount as String:"api-keys"]
        let status=SecItemUpdate(query as CFDictionary,[kSecValueData as String:data] as CFDictionary)
        if status==errSecSuccess{return true}
        if status != errSecItemNotFound{return false}
        var add=query;add[kSecValueData as String]=data;add[kSecAttrAccessible as String]=kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        return SecItemAdd(add as CFDictionary,nil)==errSecSuccess
    }
    @objc func riskSettings(){
        guard !busy else{message("本轮测试完成后可修改数据源设置。");return}
        guard let existing=readRiskKeys() else{message("钥匙串读取失败，原配置已保留。",error:true);return}
        let alert=NSAlert();alert.messageText="IP 数据源设置";alert.informativeText="IPPure 和 NCGY 直接查询。ProxyCheck 可匿名使用；填写密钥后使用账户额度。其余来源需 API 密钥，留空显示未配置。密钥保存在 Mac 钥匙串，查询只发送目标 IP。"
        alert.addButton(withTitle:"保存");alert.addButton(withTitle:"取消")
        let fields:[(String,String,Bool)]=[("ProxyCheck","ProxyCheck Key",true),("ipapi.is","ipapi.is Key",true),("scam_user","Scamalytics 用户名",false),("Scamalytics","Scamalytics Key",true),("scam_region","Scamalytics 节点 US / EU",false),("AbuseIPDB","AbuseIPDB Key",true),("IPQS","IPQualityScore Key（可选）",true)]
        let box=NSView(frame:NSRect(x:0,y:0,width:540,height:CGFloat(fields.count*40+25)));var inputs:[String:NSTextField]=[:]
        for (i,item) in fields.enumerated(){let (id,title,secure)=item;let y=CGFloat((fields.count-1-i)*40+25);let label=NSTextField(labelWithString:title);label.frame=NSRect(x:0,y:y+4,width:215,height:22);box.addSubview(label);let field:NSTextField=secure ? NSSecureTextField():NSTextField();field.frame=NSRect(x:220,y:y,width:320,height:26);field.stringValue=existing[id] ?? (id=="scam_region" ? "US":"");field.placeholderString=secure ? "填写 API 密钥":"";box.addSubview(field);inputs[id]=field}
        let note=NSTextField(labelWithString:"各家需自行申请密钥；查询失败或限流会单独显示。")
        note.frame=NSRect(x:0,y:0,width:530,height:20);note.font=NSFont.systemFont(ofSize:11);box.addSubview(note);alert.accessoryView=box
        if alert.runModal() == .alertFirstButtonReturn {var keys:[String:String]=[:];for (id,f) in inputs {let value=f.stringValue.trimmingCharacters(in:.whitespacesAndNewlines);if !value.isEmpty {keys[id]=value}}
            keys["scam_region"]=(keys["scam_region"] ?? "US").uppercased()
            if !["US","EU"].contains(keys["scam_region"] ?? ""){message("Scamalytics 节点请填写 US 或 EU，与注册时选择一致。",error:true);return}
            if saveRiskKeys(keys){message("数据源设置已保存到 Mac 钥匙串，下次测试生效。")}else{message("钥匙串保存失败，请检查系统授权。",error:true)}
        }
    }
    @objc func about(){let a=NSAlert();a.messageText="节点测评 1.3.1";a.informativeText="本地全订阅测速与解锁检测\n基于 Mihomo 与 Clash SpeedTest Pro；发行版 AGPL-3.0\n所有检测在本机执行。订阅及节点凭据仅保留在运行内存。\n源码：https://github.com/WWangZQ/node-test；许可证位于 Resources/licenses。";a.runModal()}
    @objc func showResults(){NSWorkspace.shared.open(support.appendingPathComponent("reports"))}
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication)->Bool{true}
    func applicationWillTerminate(_ n: Notification){stopWorker()}
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping(WKNavigationActionPolicy)->Void){decisionHandler(action.request.url?.isFileURL == true ? .allow : .cancel)}
    func emit(_ obj: [String:Any]) {
        guard let d=try? JSONSerialization.data(withJSONObject:obj,options:[.sortedKeys]),let s=String(data:d,encoding:.utf8) else{return}
        web.evaluateJavaScript("window.receive(\(s))",completionHandler:nil)
    }
    func message(_ text:String,error:Bool=false){emit(["type":"message","text":text,"error":error])}
    func userContentController(_ controller: WKUserContentController,didReceive message: WKScriptMessage){
        guard message.frameInfo.isMainFrame,let b=message.body as? [String:Any],let cmd=b["cmd"] as? String else{return}
        switch cmd {
        case "ready": interfaces(); history(); if let r=report{emit(["type":"report","report":r])}
        case "riskSettings":riskSettings()
        case "loadFile":openFile()
        case "loadURL":if let u=b["url"] as? String{loadURL(u)}
        case "start":start(b["options"] as? [String:Any] ?? [:])
        case "cancel":stopWorker();self.message("正在停止测试，已完成的结果会保留。")
        case "save":save(b)
        case "reference":if let text=b["url"] as? String,let url=URL(string:text),url.scheme=="https",["ipleak.net","peanutdl.com","ipinfo.cv","ip.net.coffee","ippure.com","proxycheck.io","ip.nc.gy","support.claude.com"].contains(url.host ?? ""){NSWorkspace.shared.open(url)}
        case "history":if !busy,let id=b["id"] as? String,UUID(uuidString:id) != nil,let d=try? Data(contentsOf:support.appendingPathComponent("reports/\(id).json")),let r=try? JSONSerialization.jsonObject(with:d) as? [String:Any]{report=r;emit(["type":"report","report":r])}
        default:break
        }
    }
    func interfaces(){
        var head:UnsafeMutablePointer<ifaddrs>?
        var names=Set<String>()
        if getifaddrs(&head)==0 {var p=head;while let q=p{let a=q.pointee;if let addr=a.ifa_addr,Int32(addr.pointee.sa_family)==AF_INET,(a.ifa_flags & UInt32(IFF_UP)) != 0 {let name=String(cString:a.ifa_name);if name.hasPrefix("en"){names.insert(name)}};p=a.ifa_next};freeifaddrs(head)}
        let list=names.sorted();emit(["type":"interfaces","items":list,"preferred":list.first ?? ""])
    }
    @objc func openFile(){
        guard !busy && !importing else{return}
        let p=NSOpenPanel();p.title="导入订阅文件";p.canChooseDirectories=false;p.allowsMultipleSelection=false
        p.beginSheetModal(for:window){[weak self] r in guard r == .OK,let u=p.url,let self=self else{return};do{let size=(try u.resourceValues(forKeys:[.fileSizeKey])).fileSize ?? 0;if size>5_000_000{self.message("文件超过 5 MB，请精简后导入。",error:true);return};let text=try String(contentsOf:u,encoding:.utf8);self.parse(text,name:u.lastPathComponent)}catch{self.message("文件读取失败，请使用 UTF-8 文本订阅文件。",error:true)}}
    }
    func loadURL(_ text:String){
        guard !busy && !importing else{return}
        guard let u=URL(string:text),["https","http"].contains(u.scheme?.lowercased() ?? ""),u.host != nil else{message("订阅链接无效。",error:true);return}
        importing=true;message("正在读取订阅…")
        loader=SubscriptionLoader(url:u){[weak self] text in
            DispatchQueue.main.async{guard let self=self else{return};self.loader=nil;self.importing=false;if let text=text{self.parse(text,name:u.host ?? "订阅")}else{self.message("订阅读取失败。请检查链接、网络，或下载订阅文件后导入（最大 5 MB）。",error:true)}}
        }
    }
    func execute(_ input:[String:Any],track:Bool=true)->[String:Any]?{
        let p=Process();p.executableURL=worker;p.currentDirectoryURL=support
        let out=Pipe(),inPipe=Pipe();p.standardInput=inPipe;p.standardOutput=out;p.standardError=FileHandle.nullDevice
        guard let data=try? JSONSerialization.data(withJSONObject:input) else{return nil}
        lock.lock();if track && cancelled{lock.unlock();return nil};if track{current=p};do{try p.run()}catch{current=nil;lock.unlock();return nil};lock.unlock()
        // Write concurrently to avoid pipe-buffer deadlocks for large subscriptions.
        DispatchQueue.global().async{try? inPipe.fileHandleForWriting.write(contentsOf:data);try? inPipe.fileHandleForWriting.close()}
        let output=out.fileHandleForReading.readDataToEndOfFile();p.waitUntilExit()
        lock.lock();if current===p{current=nil};lock.unlock()
        guard let s=String(data:output,encoding:.utf8),let line=s.split(separator:"\n").last(where:{$0.hasPrefix("RESULT:")}),let d=String(line.dropFirst(7)).data(using:.utf8) else{return nil}
        return try? JSONSerialization.jsonObject(with:d) as? [String:Any]
    }
    func parse(_ text:String,name:String){
        importing=true;message("正在解析节点…")
        queue.async{let result=self.execute(["action":"parse","content":text],track:false)
            DispatchQueue.main.async{self.importing=false;guard let ns=result?["nodes"] as? [[String:Any]],!ns.isEmpty else{self.message(result?["error"] as? String ?? "订阅解析失败。",error:true);return};self.nodes=ns;self.sourceName=String(name.prefix(100));let rows=ns.enumerated().map{(i,n)->[String:Any] in ["name":String(String(describing:n["name"] ?? "节点 \(i+1)").prefix(200)),"type":String(describing:n["type"] ?? "unknown"),"status":"pending","services":[:],"samples":[]]};self.report=["id":UUID().uuidString,"name":self.sourceName,"status":"ready","demo":false,"created":self.date(),"rows":rows];self.emit(["type":"report","report":self.report!]);self.emit(["type":"source","count":ns.count])}
        }
    }
    func date()->String{let f=DateFormatter();f.dateFormat="yyyy-MM-dd HH:mm:ss";return f.string(from:Date())}
    func start(_ options:[String:Any]){
        guard !busy && !importing && !nodes.isEmpty else{return}
        let mode=options["mode"] as? String ?? "both"
        guard ["both","speed","unlock"].contains(mode) else{return}
        let seconds=max(2,min(15,options["seconds"] as? Int ?? 8));let mb=max(1,min(256,options["megabytes"] as? Int ?? 64))
        let iface=options["interface"] as? String ?? ""
        guard iface.isEmpty || iface.range(of:"^en[0-9]+$",options:.regularExpression) != nil else{return}
        let platforms=Array(Set(options["platforms"] as? [String] ?? [])).sorted().filter{$0.count<80}
        guard platforms.count<=60 else{return}
        let risk=options["ip_risk"] as? Bool ?? true
        let settings:[String:Any]=["mode":mode,"seconds":seconds,"megabytes":mb,"timeout":8,"ip_info":true,"interface":iface,"platforms":platforms,"ip_risk":risk]
        guard let riskKeys = (risk ? readRiskKeys():[:]) else {message("无法读取 IP 数据源钥匙串，请允许访问后重试。",error:true);return}
        let ns=nodes;busy=true;lock.lock();cancelled=false;lock.unlock()
        var rows=ns.enumerated().map{(i,n)->[String:Any] in ["name":String(String(describing:n["name"] ?? "节点 \(i+1)").prefix(200)),"type":String(describing:n["type"] ?? "unknown"),"status":"pending","services":[:],"samples":[]]}
        var r:[String:Any]=["id":UUID().uuidString,"name":sourceName,"status":"running","demo":false,"created":date(),"rows":rows,"settings":settings,"units":["speed":"MB/s","latency":"ms","bytes":"bytes"],"probe_version":"1.3","engine":"mihomo 1.19.10 / Clash SpeedTest Pro 5d86384 + local worker"]
        report=r;emit(["type":"report","report":r]);message("正在逐个测试节点，完成一行就会更新一行。")
        queue.async{
            var riskCache:[String:Any]=[:]
            var riskDisabled:[String:String]=[:]
            let defaults=UserDefaults.standard
            let day=String(self.date().prefix(10))
            if defaults.string(forKey:"ProxyCheck.day") != day {defaults.set(day,forKey:"ProxyCheck.day");defaults.set(0,forKey:"ProxyCheck.used")}
            let dailyBudget=(riskKeys["ProxyCheck"] ?? "").isEmpty ? 100:1000
            var remaining=max(0,dailyBudget-defaults.integer(forKey:"ProxyCheck.used"))
            for (i,node) in ns.enumerated(){
                self.lock.lock();let stop=self.cancelled;self.lock.unlock();if stop{break}
                rows[i]["status"]="running";r["rows"]=rows
                let progress=r;DispatchQueue.main.async{self.report=progress;self.emit(["type":"report","report":progress])}
                var input=settings;input["node"]=node;input["action"]="test";input["risk_cache"]=riskCache;input["proxycheck_remaining"]=remaining;input["risk_keys"]=riskKeys;input["risk_disabled"]=riskDisabled
                let result=self.execute(input)
                self.lock.lock();let stopped=self.cancelled;self.lock.unlock()
                if stopped{rows[i]["status"]="cancelled";rows[i]["error"]="用户停止测试";break}
                if let result=result{
                    let requests=result["proxycheck_requests"] as? Int ?? 0
                    remaining=max(0,remaining-requests)
                    defaults.set(defaults.integer(forKey:"ProxyCheck.used")+requests,forKey:"ProxyCheck.used")
                    if result["proxycheck_limited"] as? Bool == true {remaining=0;defaults.set(dailyBudget,forKey:"ProxyCheck.used")}
                    if let stopped=result["risk_stopped"] as? [String:String] {for (name,status) in stopped {riskDisabled[name]=status}}
                    if let updates=result["risk_updates"] as? [String:Any] {for (ip,value) in updates{riskCache[ip]=value}}
                    for (key,value) in result where key != "risk_updates" && key != "risk_stopped" {rows[i][key]=value}
                }else{rows[i]["status"]="failed";rows[i]["error"]="检测进程退出，已跳过此节点"}
                r["rows"]=rows;self.persist(r)
                let update=r;DispatchQueue.main.async{self.report=update;self.emit(["type":"report","report":update])}
            }
            self.lock.lock();let stopped=self.cancelled;self.lock.unlock()
            if stopped{for i in rows.indices{if rows[i]["status"] as? String == "pending"{rows[i]["status"]="cancelled"}}}
            r["rows"]=rows;r["status"]=stopped ? "cancelled":"complete";r["finished"]=self.date();self.persist(r)
            let final=r;DispatchQueue.main.async{self.busy=false;self.report=final;self.emit(["type":"report","report":final]);self.message(stopped ? "已停止，已完成的结果可以导出。":"测试完成。结果已保存在本机，可保存长图或导出表格。");self.history()}
        }
    }
    func stopWorker(){lock.lock();cancelled=true;if let p=current,p.isRunning{p.terminate()};lock.unlock()}
    func persist(_ r:[String:Any]){guard let id=r["id"] as? String,let d=try? JSONSerialization.data(withJSONObject:r,options:[.prettyPrinted,.sortedKeys]) else{return};try? d.write(to:support.appendingPathComponent("reports/\(id).json"),options:.atomic)}
    func history(){let dir=support.appendingPathComponent("reports");let files=(try? FileManager.default.contentsOfDirectory(at:dir,includingPropertiesForKeys:nil)) ?? [];var items:[[String:String]]=[];for f in files where f.pathExtension=="json"{if let d=try? Data(contentsOf:f),var r=try? JSONSerialization.jsonObject(with:d) as? [String:Any],let id=r["id"] as? String{if !busy && r["status"] as? String == "running"{r["status"]="interrupted";if var rows=r["rows"] as? [[String:Any]]{for i in rows.indices{if ["pending","running"].contains(rows[i]["status"] as? String ?? ""){rows[i]["status"]="interrupted"}};r["rows"]=rows};persist(r)};items.append(["id":id,"name":r["name"] as? String ?? "订阅","created":r["created"] as? String ?? ""])}};items.sort{($0["created"] ?? "") > ($1["created"] ?? "")};emit(["type":"history","items":items])}
    func save(_ b:[String:Any]){
        guard let format=b["format"] as? String,["png","csv","json","html"].contains(format),let content=b["content"] as? String,content.utf8.count<100_000_000 else{return}
        let data=format=="png" ? Data(base64Encoded:content):content.data(using:.utf8)
        guard let data=data else{message("导出内容无效。",error:true);return}
        let p=NSSavePanel();p.title="保存测试结果";p.nameFieldStringValue=(b["name"] as? String ?? "节点测评")+"."+format;p.allowedContentTypes=[UTType(filenameExtension:format) ?? .data];p.canCreateDirectories=true
        p.beginSheetModal(for:window){[weak self] r in guard r == .OK,let u=p.url else{return};do{try data.write(to:u,options:.atomic);self?.message("已保存：\(u.lastPathComponent)")}catch{self?.message("保存失败，请选择可写入的文件夹。",error:true)}}
    }
}
let app=NSApplication.shared
let delegate=App()
app.delegate=delegate
app.setActivationPolicy(.regular)
app.run()
