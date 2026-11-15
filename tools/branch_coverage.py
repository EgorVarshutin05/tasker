#!/usr/bin/env python3
"""Measure decision outcomes in a disposable instrumented source copy."""
import json,os,shutil,subprocess,tempfile
from pathlib import Path
root=Path(__file__).resolve().parents[1]
out=root/'reports/branches';out.mkdir(parents=True,exist_ok=True)
events=out/'events'
if events.exists():shutil.rmtree(events)
events.mkdir()
manifest=out/'manifest.json'
with tempfile.TemporaryDirectory(prefix='tasks-branches-') as temp:
    copy=Path(temp)/'project'
    shutil.copytree(root,copy,ignore=shutil.ignore_patterns('node_modules','reports','bin','dist','.git','test-results','playwright-report','__pycache__','*.db','*.db-wal','*.db-shm'))
    probe=copy/'internal/branchprobe/probe.go';probe.parent.mkdir(parents=True)
    probe.write_text((root/'tools/branchcover/probe.txt').read_text())
    subprocess.run(['go','run','-buildvcs=false','./tools/branchcover','-root',str(copy),'-manifest',str(manifest)],cwd=root,check=True)
    env=dict(os.environ,BRANCH_DIR=str(events))
    with (out/'tests.log').open('w') as log:
        subprocess.run(['go','test','-race','-count=1','-v','./internal/...'],cwd=copy,env=env,stdout=log,stderr=subprocess.STDOUT,check=True)
seen=set()
for file in events.glob('*.jsonl'):
    for line in file.read_text().splitlines():
        event=json.loads(line);seen.add((event['id'],event['outcome']))
rows=json.loads(manifest.read_text());covered=total=service_covered=service_total=0
for branch in rows:
    branch['executed']=[value for value in branch['outcomes'] if (branch['id'],value) in seen]
    branch['missing']=[value for value in branch['outcomes'] if (branch['id'],value) not in seen]
    total+=len(branch['outcomes']);covered+=len(branch['executed'])
    if branch['file'].startswith('internal/service/'):
        service_total+=len(branch['outcomes']);service_covered+=len(branch['executed'])
result={'scope':'Explicit if/conditional-for decisions and switch case/default outcomes. Range-loop iteration and MC/DC are not measured.',
        'covered':covered,'total':total,'percent':100*covered/total,
        'service_covered':service_covered,'service_total':service_total,'service_percent':100*service_covered/service_total,
        'branches':rows}
result['passed']=result['percent']>=80 and result['service_percent']==100
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n')
print(f"Decision outcomes: {covered}/{total} = {result['percent']:.2f}%; service {service_covered}/{service_total} = {result['service_percent']:.2f}%")
for branch in rows:
    if branch['missing']:print('Missing:',branch['id'],branch['missing'])
raise SystemExit(0 if result['passed'] else 1)
