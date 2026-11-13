#!/usr/bin/env python3
"""Real CLI lifecycle and HTTP/MCP integration, isolated from device state."""
import json, os, pathlib, socket, subprocess, tempfile, urllib.request, urllib.error, sys, shutil
root=pathlib.Path(__file__).resolve().parents[1]
def freeport():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0));return s.getsockname()[1]
with tempfile.TemporaryDirectory(prefix='magicap-smoke-') as d:
    binary=pathlib.Path(d)/'magicap'
    subprocess.run(['go','build','-o',str(binary),'./cmd/magicap'],cwd=root,check=True)
    module=pathlib.Path(d)/'module';shutil.copytree(root/'src/magicap/webroot',module/'webroot')
    env={**os.environ,'MAGICAP_STATE':d+'/state','MAGICAP_ROOT':str(module)}
    def cli(*args):return subprocess.check_output([str(binary),*args],env=env,text=True)
    web,proxy,mcp=freeport(),freeport(),freeport()
    def request(path,data=None,method=None,extra=None):
        h={'Content-Type':'application/json',**(extra or {})}
        r=urllib.request.Request(f'http://127.0.0.1:{web}'+path,data=None if data is None else json.dumps(data).encode(),headers=h,method=method)
        return urllib.request.urlopen(r,timeout=10)
    cli('config','--web-port',str(web),'--proxy-port',str(proxy),'--mcp-port',str(mcp))
    assert not json.loads(cli('mcp','status','--json'))['enabled']
    cert=cli('cert','init').strip()
    expected=subprocess.check_output(['openssl','x509','-in',cert,'-subject_hash_old','-noout'],text=True).strip()+'.0'
    assert cli('cert','overlay').strip()==expected
    try:
        cli('capture','enable');cli('capture','enable')
        request('/api/rules',[{'match':'magicap.test/mock','mock_status':201,'mock_body':'hello'}],'PUT').close()
        out=json.load(request('/api/replay',{'url':'http://magicap.test/mock'},'POST'));assert out['status']==201
        flows=json.load(request('/api/sessions'));assert len(flows)==1 and flows[0]['status']==201
        assert json.load(request('/api/har'))['log']['version']=='1.2'
        assert b'Magicap' in request('/').read()
        try:request('/api/sessions',extra={'Origin':'https://evil.invalid'})
        except urllib.error.HTTPError as e:assert e.code==403
        else:raise AssertionError('cross-origin request accepted')
        cli('mcp','enable','--port',str(mcp))
        status=json.loads(cli('mcp','status','--json'));assert status['running'] and status['port']==mcp
        def rpc(method,params,id=1):
            r=urllib.request.Request(f'http://127.0.0.1:{mcp}/mcp',data=json.dumps({'jsonrpc':'2.0','id':id,'method':method,'params':params}).encode(),headers={'Content-Type':'application/json','Accept':'application/json, text/event-stream'})
            body=urllib.request.urlopen(r,timeout=10).read().decode()
            if body.startswith('event:') or body.startswith('data:'):
                body=next(line[5:].strip() for line in body.splitlines() if line.startswith('data:'))
            return json.loads(body)
        init=rpc('initialize',{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'smoke','version':'1'}});assert 'result' in init,init
        tools=rpc('tools/list',{});assert 'sessions_list' in [t['name'] for t in tools['result']['tools']],tools
        call=rpc('tools/call',{'name':'sessions_list','arguments':{}});assert not call['result'].get('isError'),call
        call=rpc('tools/call',{'name':'request_replay','arguments':{'url':'http://magicap.test/mock'}});assert not call['result'].get('isError'),call
        call=rpc('tools/call',{'name':'rules_set','arguments':{'rules':[{'match':'magicap.test/mcp','block':True}]}});assert not call['result'].get('isError'),call
        assert json.load(request('/api/rules'))[0]['match']=='magicap.test/mcp'
        cli('config','--secret','smoke-secret')
        assert b'Magicap' in request('/').read()
        try:request('/api/sessions')
        except urllib.error.HTTPError as e:assert e.code==401
        else:raise AssertionError('missing auth accepted')
        assert json.load(request('/api/sessions',extra={'Authorization':'Bearer smoke-secret'}))==[]
        cli('mcp','disable');cli('capture','disable')
        assert not json.loads(cli('status'))['running']
        print('PASS: CA hash, idempotent CLI, proxy/mock/replay, HAR, WebUI, Origin/auth, MCP initialize/list/call/disable')
    finally:
        cli('stop')
