#!/usr/bin/env python3
import datetime,json,os,platform,subprocess,hashlib
from pathlib import Path
root=Path(__file__).resolve().parents[1]
def command(args):
    try:return subprocess.check_output(args,cwd=root,stderr=subprocess.STDOUT,text=True).strip()
    except (OSError,subprocess.CalledProcessError) as e:return str(e)
result={'timestamp':datetime.datetime.now(datetime.timezone.utc).isoformat(),'platform':platform.platform(),
        'cpu_count':os.cpu_count(),'go':command(['go','version']),'embedded_sqlite':command(['go','run','-buildvcs=false','./tools/versions']),'node':command(['node','--version']),
        'npm':command(['npm','--version']),'k6':command(['k6','version']),
        'commit':command(['git','rev-parse','HEAD']),'cgo_enabled':command(['go','env','CGO_ENABLED']),
        'gcc':command(['gcc','--version']),
        'sqlite_driver':command(['go','list','-m','github.com/mattn/go-sqlite3']),
        'playwright':command(['node','-p',"require('./e2e/node_modules/@playwright/test/package.json').version"]),
        'plan_sha256':hashlib.sha256((root/'docs/task_test_plan.tex').read_bytes()).hexdigest()}
reports=root/'reports';reports.mkdir(exist_ok=True)
(reports/'environment.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
