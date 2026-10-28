package service

// Python 3.6 syntax is intentional: it is the Python 3 shipped by CentOS 7.9.
const deployProbePython = `import os, sys, json, platform, subprocess, shutil, urllib.request, base64, gzip, re, ipaddress
import xml.etree.ElementTree as ET

def read_networks():
    try:
        interfaces=json.loads(subprocess.check_output(['ip','-j','-4','address'],universal_newlines=True,stderr=subprocess.DEVNULL))
    except (subprocess.CalledProcessError,ValueError):
        by_name={}
        for line in subprocess.check_output(['ip','-o','-4','address','show'],universal_newlines=True).splitlines():
            parts=line.split()
            if 'inet' not in parts or len(parts)<4: continue
            name=parts[1].split('@')[0]; address=parts[parts.index('inet')+1].split('/')
            scope=parts[parts.index('scope')+1] if 'scope' in parts else 'global'
            by_name.setdefault(name,dict(ifname=name,addr_info=[]))['addr_info'].append(dict(local=address[0],prefixlen=int(address[1]),scope=scope))
        interfaces=list(by_name.values())
    networks=[str(ipaddress.ip_network(a['local']+'/'+str(a['prefixlen']),strict=False)) for i in interfaces for a in i.get('addr_info',[]) if a.get('scope')=='global']
    try:
        routes=json.loads(subprocess.check_output(['ip','-j','-4','route'],universal_newlines=True,stderr=subprocess.DEVNULL))
        networks += [r['dst'] for r in routes if r.get('dst') and r['dst']!='default']
    except (subprocess.CalledProcessError,ValueError):
        for line in subprocess.check_output(['ip','-4','route','show'],universal_newlines=True).splitlines():
            parts=line.split()
            if not parts: continue
            try: networks.append(str(ipaddress.ip_network(parts[0],strict=False)))
            except ValueError: pass
    return interfaces,networks

def kernel_errors(system,kernel,plugin):
    match=re.match(r'^(\d+)\.(\d+)',kernel)
    if not match: return ['无法识别 Linux 内核版本']
    major,minor=map(int,match.groups())
    if not (major>=6 or (major==5 and minor in (4,10,15))):
        return ['内核 '+kernel+' 不在当前 Kubernetes 安装器支持范围；CentOS 7.9 原厂 3.10 内核不能部署当前版本，请先升级至 5.10/5.15 或 6.x 并重启后重新预检']
    if (system=='centos' or plugin=='calico') and (major,minor)<(5,10):
        return ['CentOS 7.9 / 当前 Calico 模板要求 5.10/5.15 或 6.x 内核；不会自动升级内核或重启']
    return []

def package_available(system,version,arch):
    minor='.'.join(version.split('.')[:2])
    if system!='centos':
        with urllib.request.urlopen('https://mirrors.aliyun.com/kubernetes-new/core/stable/v'+minor+'/deb/Packages.gz',timeout=25) as r: packages=gzip.decompress(r.read(16*1024*1024)).decode()
        return any(line.startswith('Version: '+version+'-') for line in packages.splitlines())
    base='https://mirrors.aliyun.com/kubernetes-new/core/stable/v'+minor+'/rpm/'
    with urllib.request.urlopen(base+'repodata/repomd.xml',timeout=25) as r: metadata=ET.fromstring(r.read(2*1024*1024))
    ns='{http://linux.duke.edu/metadata/repo}'; href=''
    for data in metadata.findall(ns+'data'):
        if data.get('type')=='primary': href=data.find(ns+'location').get('href','')
    if not href.startswith('repodata/') or '..' in href: return False
    with urllib.request.urlopen(base+href,timeout=25) as r: raw=r.read(16*1024*1024)
    if href.endswith('.gz'): raw=gzip.decompress(raw)
    if len(raw)>64*1024*1024: return False
    root=ET.fromstring(raw); common='{http://linux.duke.edu/metadata/common}'
    for package in root.findall(common+'package'):
        if package.findtext(common+'name')=='kubeadm' and package.findtext(common+'arch')==arch and package.find(common+'version').get('ver')==version: return True
    return False

def time_synchronized():
    if os.path.exists('/run/systemd/timesync/synchronized'): return True
    commands=[(['timedatectl','status'],r'(NTP synchronized|System clock synchronized):\s*yes'),(['chronyc','tracking'],r'Leap status\s*:\s*Normal'),(['ntpq','-pn'],r'(?m)^\*')]
    for command,pattern in commands:
        if not shutil.which(command[0]): continue
        try:
            output=subprocess.check_output(command,universal_newlines=True,stderr=subprocess.DEVNULL)
            if re.search(pattern,output): return True
        except (subprocess.CalledProcessError,OSError): pass
    return False

p=json.loads(base64.b64decode(sys.argv[1])); errors=[]
release={}
for line in open('/etc/os-release'):
    if '=' in line:
        k,v=line.strip().split('=',1); release[k]=v.strip('"')
system=release.get('ID',''); version=release.get('VERSION_ID',''); arch=platform.machine(); kernel=platform.release()
if system=='centos' and os.path.exists('/etc/centos-release') and re.search(r'release 7\.9\.2009(?:\s|$)',open('/etc/centos-release').read()): version='7.9'
if not ((system=='ubuntu' and version in ('22.04','24.04')) or (system=='debian' and version in ('12','13')) or (system=='centos' and version=='7.9')): errors.append('不支持的系统：'+system+' '+version)
if arch not in ('x86_64','aarch64') or (system=='centos' and arch!='x86_64'): errors.append('不支持的架构：'+arch)
manager='yum' if system=='centos' else 'apt-get'
if not shutil.which(manager) or not shutil.which('ip') or not shutil.which('systemctl'): errors.append('缺少包管理器、iproute 或 systemd')
errors+=kernel_errors(system,kernel,p['plugin'])
cpu=os.cpu_count() or 0
memory=int(next((l.split()[1] for l in open('/proc/meminfo') if l.startswith('MemTotal:')),0))//1024
disk=shutil.disk_usage('/').free//(1024**3)
if cpu<2 or memory<1800 or disk<15: errors.append('至少需要 2 CPU、约 2 GiB 内存和 15 GiB 可用磁盘')
for target in ('/etc/kubernetes/admin.conf','/etc/kubernetes/kubelet.conf','/etc/kubernetes/manifests/kube-apiserver.yaml','/var/lib/etcd/member'):
    if os.path.exists(target): errors.append('发现现有 Kubernetes/etcd 数据，禁止自动覆盖'); break
for service in ('docker','containerd','kubelet'):
    if subprocess.run(['systemctl','is-active','--quiet',service],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0: errors.append('已有运行中的 '+service+'，请使用干净的目标主机')
for target in set(['/var/lib/containerd',p['runtimeDirectory']]):
    if target and os.path.isdir(target) and os.listdir(target): errors.append('运行时目录非空：'+target)
addresses=[]; networks=[]
try:
    interfaces,networks=read_networks()
    if p['interface']=='auto': p['interface']=next((i['ifname'] for i in interfaces if any(a.get('local')==p['ip'] for a in i.get('addr_info',[]))), '')
    addresses=[a['local'] for i in interfaces if i['ifname']==p['interface'] for a in i.get('addr_info',[])]
    if p['ip'] not in addresses: errors.append('部署 IP 不属于指定网口 '+p['interface'])
    if p['master'] and p['builtinLB']:
        own=[ipaddress.ip_network(a['local']+'/'+str(a['prefixlen']),strict=False) for i in interfaces if i['ifname']==p['interface'] for a in i.get('addr_info',[])]
        if not any(ipaddress.ip_address(p['vip']) in subnet for subnet in own): errors.append('VIP 必须属于控制平面网口所在子网')
        if subprocess.run(['ping','-c','1','-W','2',p['vip']],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0: errors.append('VIP 已有 ICMP 响应，请确认未被占用')
except Exception: errors.append('无法读取网口及路由信息')
cgroup='v2' if os.path.exists('/sys/fs/cgroup/cgroup.controllers') else 'v1'
if cgroup=='v1':
    if p.get('requireV2'): errors.append('目标集群 kubelet 配置禁止 cgroup v1，此节点需使用 cgroup v2')
    if not all(os.path.isdir('/sys/fs/cgroup/'+controller) for controller in ('cpu','cpuacct','cpuset','devices','freezer','memory','pids')): errors.append('cgroup v1 控制器缺失，请检查内核和挂载配置')
if system=='centos':
    if shutil.which('getenforce') and subprocess.check_output(['getenforce'],universal_newlines=True).strip()=='Enforcing' and not p.get('disableSELinux'): errors.append('CentOS 7.9 当前安装模板需关闭 SELinux，请在配置中明确开启此选项')
    if subprocess.run(['systemctl','is-active','--quiet','firewalld'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0 and not p.get('disableFirewalld'): errors.append('Firewalld 正在运行，请核对网络要求后在配置中选择关闭，或先自行完成防火墙处理')
try:
    import socket
    ports=[10250]+([6443,2379,2380] if p['master'] else [])+([p['apiPort']] if p['master'] and p['builtinLB'] else [])
    for port in ports:
        sock=socket.socket()
        try: sock.bind(('0.0.0.0',port))
        except OSError: errors.append('端口已被占用：'+str(port))
        finally: sock.close()
except Exception: errors.append('端口检查失败')
try:
    if not package_available(system,p['version'],arch): errors.append('官方软件源未找到 Kubernetes '+p['version']+' 的目标架构包')
except Exception: errors.append('无法读取 Kubernetes 软件源，请检查出网、DNS、证书和代理')
urls=[] if p.get('joining') else ['https://raw.githubusercontent.com/projectcalico/calico/v3.31.2/manifests/calico.yaml' if p['plugin']=='calico' else 'https://raw.githubusercontent.com/flannel-io/flannel/v0.27.4/Documentation/kube-flannel.yml']
if system=='centos': urls+=['https://github.com/containerd/containerd/releases/download/v1.7.36/containerd-static-1.7.36-linux-amd64.tar.gz','https://github.com/opencontainers/runc/releases/download/v1.3.6/runc.amd64']
if p['metrics']: urls.append('https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.0/components.yaml')
for url in urls:
    try:
        with urllib.request.urlopen(url,timeout=20) as response: response.read(128)
    except Exception: errors.append('无法访问组件安装文件：'+url)
try:
    req=urllib.request.Request('https://'+p['repository'].split('/')[0]+'/v2/',method='GET')
    try: urllib.request.urlopen(req,timeout=15).close()
    except urllib.error.HTTPError as e:
        if e.code!=401: raise
except Exception: errors.append('无法访问镜像仓库；组件镜像是否存在将在拉取阶段验证')
if not time_synchronized(): errors.append('时间同步尚未成功，已尝试准备并启动同步服务，请核对 NTP 服务器和 UDP 123 网络')
machine=open('/etc/machine-id').read().strip() if os.path.exists('/etc/machine-id') else ''
print(json.dumps(dict(os=system,version=version,arch=arch,kernel=kernel,cgroup=cgroup,hostname=platform.node().split('.')[0],interface=p['interface'],machineId=machine,cpu=cpu,memoryMB=memory,diskGB=disk,addresses=addresses,networks=networks,errors=errors)))`
