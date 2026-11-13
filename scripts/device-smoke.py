#!/usr/bin/env python3
"""Non-root Android binary smoke test. Does not install a module or modify trust."""
import json, pathlib, subprocess, time, urllib.request
root=pathlib.Path(__file__).resolve().parents[1]
remote='/data/local/tmp/magicap-smoke-'+str(time.time_ns())
ports={}
if not any(line.endswith('\tdevice') for line in subprocess.check_output(['adb','devices'],text=True).splitlines()):
    raise SystemExit('No Android device connected; no device files changed.')
def adb(*args):return subprocess.check_output(['adb',*args],text=True).strip()
def cli(*args):return adb('shell',f'MAGICAP_STATE={remote}/state',f'MAGICAP_ROOT={remote}',remote+'/bin/magicap-cli',*args)
def request(port,path,data=None,headers=None):
    return urllib.request.urlopen(urllib.request.Request(f'http://127.0.0.1:{port}'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**(headers or {})}),timeout=10)
try:
    adb('shell','mkdir','-p',remote+'/bin')
    for name in ['magicap-arm64','magicap-cli']:
        adb('push',str(root/'src/magicap/bin'/name),remote+'/bin/'+name)
    adb('push',str(root/'src/magicap/lib'),remote+'/lib')
    adb('push',str(root/'src/magicap/.config'),remote+'/.config')
    adb('push',str(root/'src/magicap/service.sh'),remote+'/service.sh')
    adb('push',str(root/'src/magicap/webroot'),remote+'/webroot')
    adb('shell','chmod','755',remote+'/bin/magicap-cli',remote+'/bin/magicap-arm64')
    adb('shell','ln','-s','bin/magicap-cli',remote+'/cli')
    assert not json.loads(cli('mcp','status','--json'))['enabled']
    cli('config','--proxy-port','28080','--web-port','28081','--mcp-port','28765')
    cli('cert','overlay')
    adb('shell',f'MAGICAP_STATE={remote}/state','sh',remote+'/service.sh')
    assert not json.loads(cli('status'))['running'],'default-off boot launched daemon'
    cli('capture','enable');cli('mcp','enable','--port','28765')
    for port in [28081,28765]:ports[port]=int(adb('forward','tcp:0',f'tcp:{port}'))
    assert json.load(request(ports[28081],'/health'))['capture']
    cli('rules',"'"+json.dumps([{'match':'magicap.test/mock','mock_status':202,'mock_body':'android-device-ok'}],separators=(',',':'))+"'")
    out=json.loads(cli('replay',"'"+json.dumps({'url':'http://magicap.test/mock'},separators=(',',':'))+"'"));assert out['status']==202,out
    assert len(json.loads(cli('sessions')))==1
    payload={'jsonrpc':'2.0','id':1,'method':'tools/list','params':{}}
    body=request(ports[28765],'/mcp',payload,{'Accept':'application/json, text/event-stream'}).read().decode()
    assert 'sessions_list' in body,body
    assert json.loads(cli('mcp','status','--json'))['running']
    cli('mcp','disable');cli('capture','disable');assert not json.loads(cli('status'))['running']
    print('PASS: Android '+adb('shell','getprop','ro.build.version.release')+' arm64 binary, CA overlay generation, kamfw default-off boot, CLI, proxy mock/replay, MCP, shutdown')
finally:
    try:cli('stop')
    except Exception:pass
    for port in ports.values():adb('forward','--remove',f'tcp:{port}')
    adb('shell','rm','-rf',remote)
