#!/usr/bin/env python3
"""Collect license texts for the exact pinned Go module graph."""
import json,pathlib,subprocess,os
os.environ["GOPROXY"]="https://proxy.golang.org"
root=pathlib.Path(__file__).resolve().parents[1]
stream=subprocess.check_output(['go','list','-m','-json','all'],cwd=root,text=True)
dec=json.JSONDecoder();modules=[]
while stream.strip():
    m,n=dec.raw_decode(stream.lstrip());stream=stream.lstrip()[n:];modules.append(m)
output=root/'src/magicap/licenses';output.mkdir(exist_ok=True)
lines=['# Third-party notices','','Go modules are pinned in `go.mod` and `go.sum`. License texts ship in the module ZIP under `licenses/`.','','| Module | Version | License text |','| --- | --- | --- |']
for m in modules:
    if m.get('Main'):continue
    d=m.get('Dir')
    if not d:
        info=json.loads(subprocess.check_output(['go','mod','download','-json',m['Path']+'@'+m['Version']],text=True));d=info['Dir']
    candidates=[p for p in pathlib.Path(d).iterdir() if p.is_file() and p.name.upper().startswith(('LICENSE','COPYING','NOTICE'))]
    if not candidates:raise SystemExit('Missing license: '+m['Path'])
    for p in candidates:
        name=m['Path'].replace('/','_')+'-'+p.name
        (output/name).write_bytes(p.read_bytes())
    lines.append(f"| [{m['Path']}](https://pkg.go.dev/{m['Path']}@{m['Version']}) | {m['Version']} | {', '.join(p.name for p in candidates)} |")
lines+=['','kamfw is vendored from the Kam standard template and retains its own `src/magicap/lib/kamfw/LICENSE`.','']
(root/'THIRD_PARTY.md').write_text('\n'.join(lines))
