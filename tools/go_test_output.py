#!/usr/bin/env python3
"""Render go test -json while keeping the JSONL as the source of truth."""
import json,sys
for line in sys.stdin:
    event=json.loads(line)
    if 'Output' in event:
        print(event['Output'],end='',flush=True)
