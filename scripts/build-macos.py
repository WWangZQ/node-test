"""Build a self-contained macOS app; never install over an existing application."""
from pathlib import Path
import argparse, os, platform, plistlib, shutil, subprocess
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--arch',choices=['arm64','x86_64'],default=platform.machine());args=p.parse_args()
arch=args.arch;goarch={'arm64':'arm64','x86_64':'amd64'}[arch]
app=ROOT/'dist'/f'macos-{arch}'/'节点测评.app'
if app.exists():shutil.rmtree(app)
res=app/'Contents/Resources';(app/'Contents/MacOS').mkdir(parents=True);res.mkdir(parents=True)
env={**os.environ,'GOOS':'darwin','GOARCH':goarch,'CGO_ENABLED':'0'}
def run(*cmd,**kw):subprocess.run([str(x) for x in cmd],check=True,**kw)
run(os.environ.get('GO_BINARY','go'),'build','-trimpath','-ldflags=-s -w','-o',res/'node-worker','./cmd/local-worker',cwd=ROOT/'backend',env=env)
run('swiftc','-swift-version','5','-O','-target',f'{arch}-apple-macosx13.0','-framework','Cocoa','-framework','WebKit','-framework','Security',ROOT/'native/App.swift','-o',app/'Contents/MacOS/LocalNodeTest')
shutil.copy2(ROOT/'native/Info.plist',app/'Contents/Info.plist')
shutil.copytree(ROOT/'static',res/'ui');shutil.copytree(ROOT/'LICENSES',res/'licenses')
for file in ['LICENSE','THIRD_PARTY_NOTICES.md','README.md']:shutil.copy2(ROOT/file,res/'licenses'/file)
run('iconutil','-c','icns',ROOT/'AppIcon.iconset','-o',res/'AppIcon.icns')
run('codesign','--force','--deep','--sign','-',app)
run('codesign','--verify','--deep','--strict',app)
print(app)
