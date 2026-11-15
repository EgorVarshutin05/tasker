#!/usr/bin/env python3
"""Run warmup and measurement against different, disposable database files."""
import argparse
import datetime
import json
import os
import platform
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser()
p.add_argument('--only', choices=['read', 'write', 'all'], default='all')
p.add_argument('--output', default='reports/load')
a = p.parse_args()
out = (ROOT / a.output).resolve()
out.mkdir(parents=True, exist_ok=True)
k6 = shutil.which('k6')
if not k6:
    raise SystemExit('Install k6 first; see README.md')
manifest = {
    'started_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'platform': platform.platform(), 'machine': platform.machine(),
    'cpu_count': os.cpu_count(),
    'affinity': sorted(os.sched_getaffinity(0)),
    'meminfo': Path('/proc/meminfo').read_text(),
    'cpuinfo': Path('/proc/cpuinfo').read_text(),
    'k6': subprocess.check_output([k6, 'version'], text=True).strip(),
    'go': subprocess.check_output(['go', 'version'], text=True).strip(),
    'sqlite_python': sqlite3.sqlite_version,
    'resource_note': 'Record external cgroup limits and SSD model separately before claiming the reference stand.',
}
try:
    manifest['commit'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True, stderr=subprocess.DEVNULL).strip()
except subprocess.CalledProcessError:
    manifest['commit'] = 'archive without git history'
(out / 'environment.json').write_text(json.dumps(manifest, indent=2) + '\n')

def run_server(db, log):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    env = dict(os.environ, ADDR=f'127.0.0.1:{port}', DB_PATH=str(db))
    command = [str(ROOT / 'bin/tasks')]
    if os.environ.get('SERVER_CPUS'):
        command = ['taskset', '-c', os.environ['SERVER_CPUS']] + command
    process = subprocess.Popen(command, env=env, stdout=log, stderr=log)
    url = f'http://127.0.0.1:{port}'
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError('Server exited before readiness')
        try:
            with urllib.request.urlopen(url + '/api/tasks', timeout=.5) as response:
                if response.status == 200:
                    return process, url
        except OSError:
            time.sleep(.05)
    process.terminate()
    process.wait(timeout=5)
    raise RuntimeError('Server readiness exceeded 5 seconds')

def stop(process):
    process.send_signal(signal.SIGTERM)
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
        raise RuntimeError('Server shutdown exceeded 5 seconds')

failures = []
for kind in (['read', 'write'] if a.only == 'all' else [a.only]):
    name = 'S01' if kind == 'read' else 'S02'
    script = ROOT / ('load/s01_read.js' if kind == 'read' else 'load/s02_write.js')
    with tempfile.TemporaryDirectory(prefix='tasks-load-') as tmp:
        for warmup in (True, False):
            phase = 'warmup' if warmup else 'measurement'
            db = Path(tmp) / (phase + '.db')
            summary = out / f'{name}-{phase}.json'
            with (out / f'{name}-{phase}-server.log').open('w') as log:
                process, url = run_server(db, log)
                command = [k6, 'run', '--quiet', '-e', f'BASE_URL={url}', '-e', f'SUMMARY_PATH={summary}', '-e', f'WARMUP={int(warmup)}', str(script)]
                if os.environ.get('LOAD_CPUS'):
                    command = ['taskset', '-c', os.environ['LOAD_CPUS']] + command
                try:
                    with (out / f'{name}-{phase}-k6.log').open('w') as load_log:
                        result = subprocess.run(command, cwd=ROOT, stdout=load_log, stderr=subprocess.STDOUT, timeout=80)
                finally:
                    stop(process)
            if result.returncode:
                failures.append(f'{name} {phase}: k6 exit {result.returncode}')
            if not summary.exists():
                raise RuntimeError(f'No k6 summary: {summary}')
            if warmup:
                print(f'{name} warmup complete; measurement uses a NEW database', flush=True)
                continue
            data = json.loads(summary.read_text())
            metrics = data['metrics']
            n = metrics['requests_total']['values']['count']
            n201 = metrics['created_total']['values']['count']
            with sqlite3.connect(db) as connection:
                values = connection.execute("SELECT COUNT(*),COUNT(DISTINCT id),COALESCE(MIN(id),0),COALESCE(MAX(id),0),COALESCE(SUM(title='Нагрузка' AND done=0),0) FROM tasks").fetchone()
            expected = (n201, n201, 1, n201, n201) if kind == 'write' else (0, 0, 0, 0, 0)
            checks = {
                'database': values == expected,
                'N>=2940': n >= 2940,
                'N/60>=49': n / 60 >= 49,
                'all_thresholds': all(t['ok'] for m in metrics.values() for t in m.get('thresholds', {}).values()),
            }
            validation = {'checks': checks, 'requests': n, 'N201': n201, 'rps_over_60s': n/60,
                          'p95_ms': metrics['http_req_duration']['values']['p(95)'],
                          'sql_values': values, 'sql_expected': expected,
                          'sqlite_bytes': db.stat().st_size}
            (out / f'{name}-validation.json').write_text(json.dumps(validation, indent=2) + '\n')
            print(name, json.dumps(validation), flush=True)
            if not all(checks.values()):
                failures.append(name + ': validation failed')
if failures:
    raise SystemExit('\n'.join(failures))
print('Both thresholds and independent SQL checks passed for selected runs.')
