// SPDX-License-Identifier: AGPL-3.0-only
import Cocoa

final class AppUpdater {
    var active=false
    let busy:()->Bool
    let message:(String,Bool)->Void
    init(busy:@escaping()->Bool,message:@escaping(String,Bool)->Void){self.busy=busy;self.message=message}
    var version:String{Bundle.main.object(forInfoDictionaryKey:"CFBundleShortVersionString") as? String ?? "0.0.0"}
    func run(_ arguments:[String]) throws->[String:Any]{
        let process=Process();process.executableURL=Bundle.main.resourceURL!.appendingPathComponent("node-updater");process.arguments=arguments
        let pipe=Pipe();process.standardOutput=pipe;process.standardError=FileHandle.nullDevice
        try process.run();let bytes=pipe.fileHandleForReading.readDataToEndOfFile();process.waitUntilExit()
        guard let value=(try? JSONSerialization.jsonObject(with:bytes)) as? [String:Any] else{throw NSError(domain:"Update",code:1,userInfo:[NSLocalizedDescriptionKey:"更新服务响应异常，请重试"])}
        if let error=value["error"] as? String{throw NSError(domain:"Update",code:2,userInfo:[NSLocalizedDescriptionKey:error])}
        guard process.terminationStatus==0 else{throw NSError(domain:"Update",code:3,userInfo:[NSLocalizedDescriptionKey:"更新准备失败，请重试"])}
        return value
    }
    func check(automatic:Bool=false){
        guard !active else{return}
        guard !busy() else{if !automatic{message("本轮测试完成后可更新应用。",false)};return}
        if automatic && Date().timeIntervalSince1970-UserDefaults.standard.double(forKey:"update.lastCheck")<86400{return}
        active=true;if !automatic{message("正在检查更新…",false)}
        let current=version
        DispatchQueue.global(qos:.utility).async{
            do{let result=try self.run(["check","--current",current]);DispatchQueue.main.async{
                self.active=false;UserDefaults.standard.set(Date().timeIntervalSince1970,forKey:"update.lastCheck")
                guard result["available"] as? Bool == true,let next=result["version"] as? String else{if !automatic{self.message("当前已是最新版本 "+current,false)};return}
                if self.busy(){return}
                if automatic && UserDefaults.standard.string(forKey:"update.reminded")==next{return}
                UserDefaults.standard.set(next,forKey:"update.reminded")
                let alert=NSAlert();alert.messageText="节点测评 "+next+" 已发布";alert.informativeText="下载后重启应用。测试记录与设置保留。";alert.addButton(withTitle:"下载并重启");alert.addButton(withTitle:"稍后")
                if alert.runModal() == .alertFirstButtonReturn{self.install(next)}
            }}catch{DispatchQueue.main.async{self.active=false;if !automatic{self.message(error.localizedDescription,true)}}}
        }
    }
    func install(_ next:String){
        guard !active && !busy() else{return};active=true;message("正在下载并校验更新…",false)
        let current=version,target=Bundle.main.bundleURL.path
        DispatchQueue.global(qos:.utility).async{
            do{let result=try self.run(["prepare","--current",current,"--version",next,"--app",target]);guard let plan=result["plan"] as? String else{throw NSError(domain:"Update",code:4,userInfo:[NSLocalizedDescriptionKey:"更新计划缺失"])}
                DispatchQueue.main.async{
                    let helper=URL(fileURLWithPath:plan).deletingLastPathComponent().appendingPathComponent("node-updater")
                    let p=Process();p.executableURL=helper;p.arguments=["apply","--plan",plan,"--pid",String(ProcessInfo.processInfo.processIdentifier)];p.standardOutput=FileHandle.nullDevice;p.standardError=FileHandle.nullDevice
                    do{try p.run();NSApp.terminate(nil)}catch{self.active=false;self.message("更新助手启动失败，请重试。",true)}
                }
            }catch{DispatchQueue.main.async{self.active=false;self.message(error.localizedDescription,true)}}
        }
    }
}
