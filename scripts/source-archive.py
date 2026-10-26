"""Archive reviewed tracked source plus vendored Go source, never local reports/keys."""
from pathlib import Path
import subprocess,tarfile
root=Path(__file__).resolve().parents[1];version=(root/'VERSION').read_text().strip()
if not (root/'backend/vendor/modules.txt').exists():raise SystemExit('Run go mod vendor in backend first')
(root/'dist').mkdir(exist_ok=True)
files=subprocess.check_output(['git','ls-files','-z'],cwd=root).decode().split('\0')
with tarfile.open(root/'dist'/f'NodeTest-{version}-source-with-dependencies.tar.gz','w:gz') as archive:
 for name in files:
  if name:archive.add(root/name,arcname='node-test/'+name,recursive=False)
 archive.add(root/'backend/vendor',arcname='node-test/backend/vendor')
print('Source archive created')
