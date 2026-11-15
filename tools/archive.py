#!/usr/bin/env python3
"""Build a source archive without local databases, binaries or dependencies."""
from pathlib import Path
import zipfile
root=Path(__file__).resolve().parents[1]
destination=root/'dist/tasks-testing-project.zip'
destination.parent.mkdir(exist_ok=True)
skip={'.git','node_modules','bin','reports','test-results','playwright-report','dist','__pycache__'}
with zipfile.ZipFile(destination,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
    for path in sorted(root.rglob('*')):
        if not path.is_file():continue
        relative=path.relative_to(root)
        if any(part in skip for part in relative.parts):continue
        if path.suffix in {'.db','.pyc','.aux','.toc','.out','.log'} or path.name.endswith(('.db-wal','.db-shm')):continue
        archive.write(path,Path(root.name)/relative)
with zipfile.ZipFile(destination) as archive:
    assert archive.testzip() is None
    print(f'{destination}: {len(archive.infolist())} files, {destination.stat().st_size} bytes')
