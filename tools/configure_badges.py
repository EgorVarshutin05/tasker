#!/usr/bin/env python3
import argparse
import re
from pathlib import Path
p=argparse.ArgumentParser(description='Bind dynamic badges to your GitHub repository')
p.add_argument('owner');p.add_argument('repository');a=p.parse_args()
for value in (a.owner,a.repository):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+',value):
        p.error('Use a GitHub owner/repository name, not a URL')
readme=Path(__file__).resolve().parents[1]/'README.md'
s=readme.read_text()
s=s.replace('OWNER/REPOSITORY',a.owner+'/'+a.repository)
readme.write_text(s)
print('Updated README.md badge URLs')
