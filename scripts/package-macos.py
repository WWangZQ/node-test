"""Create and verify a drag-to-Applications disk image from the built app."""
from pathlib import Path
import argparse, os, plistlib, shutil, subprocess, tempfile
root=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--arch',required=True,choices=['arm64','x86_64']);args=p.parse_args()
version=(root/'VERSION').read_text().strip()
app=root/'dist'/f'macos-{args.arch}'/'节点测评.app'
image=root/'dist'/f'NodeTest-{version}-macos-{args.arch}.dmg'
def run(*cmd):return subprocess.run([str(c) for c in cmd],check=True,capture_output=True)
with tempfile.TemporaryDirectory(prefix='node-test-dmg-') as folder:
 stage=Path(folder)/'content';stage.mkdir()
 run('ditto',app,stage/app.name)
 os.symlink('/Applications',stage/'Applications')
 run('hdiutil','create','-volname','节点测评','-srcfolder',stage,'-format','UDZO','-ov',image)
run('hdiutil','verify',image)
with tempfile.TemporaryDirectory(prefix='node-test-mount-') as folder:
 mounted=False
 try:
  run('hdiutil','attach','-readonly','-nobrowse','-mountpoint',folder,image);mounted=True
  mounted_app=Path(folder)/app.name
  assert os.readlink(Path(folder)/'Applications')=='/Applications'
  info=plistlib.loads((mounted_app/'Contents/Info.plist').read_bytes())
  assert info['CFBundleShortVersionString']==version
  run('codesign','--verify','--deep','--strict',mounted_app)
 finally:
  if mounted:run('hdiutil','detach',folder)
print(f'Created and verified {image.name}')
