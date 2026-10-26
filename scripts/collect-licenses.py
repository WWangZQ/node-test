from pathlib import Path
import shutil
root=Path(__file__).resolve().parents[1]
v=root/'backend/vendor';target=root/'LICENSES/go'
if not (v/'modules.txt').exists():raise SystemExit('go mod vendor is required')
for p in v.rglob('*'):
 if p.is_file() and p.name.lower().startswith(('license','copying','copyright','notice','authors','patents')):
  dest=target/p.relative_to(v);dest.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,dest)
shutil.copy2(v/'modules.txt',root/'LICENSES/go-modules.txt')
print('Collected Go dependency notices')
