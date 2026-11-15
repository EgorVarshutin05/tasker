#!/usr/bin/env python3
"""Enforce weighted statement coverage; never call it branch coverage."""
import argparse
import json
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument('profile', nargs='?', default='reports/coverage.out')
p.add_argument('--total', type=float, default=85)
p.add_argument('--service', type=float, default=90)
a = p.parse_args()
counts = {}
blocks = {}
for line in Path(a.profile).read_text().splitlines():
    if line.startswith('mode:'):
        continue
    location, statements, executions = line.rsplit(None, 2)
    package = next((name for name in ('service', 'repository', 'handler') if '/internal/' + name + '/' in location), None)
    if package is None:
        raise SystemExit('Unexpected coverage package: ' + location)
    n = int(statements)
    previous = blocks.setdefault(location, [package, n, 0])
    if previous[1] != n:
        raise SystemExit('Statement count conflict: ' + location)
    previous[2] += int(executions)
# -coverpkg repeats blocks across package test binaries; Go merges them.
for package, n, executions in blocks.values():
    covered, total = counts.setdefault(package, [0, 0])
    counts[package] = [covered + (n if executions > 0 else 0), total + n]
if set(counts) != {'service', 'repository', 'handler'}:
    raise SystemExit('Missing package in coverage profile')
result = {key: {'covered': value[0], 'total': value[1], 'percent': 100 * value[0] / value[1]} for key, value in counts.items()}
covered = sum(value[0] for value in counts.values())
total = sum(value[1] for value in counts.values())
result['total'] = {'covered': covered, 'total': total, 'percent': 100 * covered / total}
result['thresholds'] = {'total': a.total, 'service': a.service}
passed = result['total']['percent'] >= a.total and result['service']['percent'] >= a.service
result['passed'] = passed
out = Path(a.profile).with_name('coverage-summary.json')
out.write_text(json.dumps(result, indent=2) + '\n')
for name in ('service', 'repository', 'handler', 'total'):
    value = result[name]
    print(f"{name}: {value['covered']}/{value['total']} = {value['percent']:.2f}%")
raise SystemExit(0 if passed else 1)
