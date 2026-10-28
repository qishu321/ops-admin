package service

import (
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const yumSourceFixture = `[ops-admin-centos7-custom]
name=Aliyun CentOS
baseurl=https://mirrors.aliyun.com/centos-vault/7.9.2009/os/$basearch/
enabled=0
gpgcheck=1
sslverify=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7
`

func expandedInstallScript(script string) string {
	result := script
	for _, match := range regexp.MustCompile(`printf %s '([A-Za-z0-9+/=]+)'`).FindAllStringSubmatch(script, -1) {
		decoded, e := base64.StdEncoding.DecodeString(match[1])
		if e == nil {
			result += "\n" + string(decoded)
		}
	}
	return result
}

func TestDeployYumSourceValidationKeepsSignaturesAndScope(t *testing.T) {
	if e := validateDeployCustomSource(yumSourceFixture); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct{ from, to string }{{"gpgcheck=1", "gpgcheck=0"}, {"sslverify=1", "sslverify=0"}, {"enabled=0", "enabled=1"}, {"https://mirrors", "http://mirrors"}, {"ops-admin-centos7-custom", "base"}, {"name=Aliyun CentOS", "proxy_password=secret"}} {
		if validateDeployCustomSource(strings.ReplaceAll(yumSourceFixture, change.from, change.to)) == nil {
			t.Fatalf("accepted unsafe repo change: %s", change.to)
		}
	}
	if e := validateDeployCustomSource("deb https://mirrors.aliyun.com/ubuntu noble main universe"); e != nil {
		t.Fatal(e)
	}
}

func TestDeployPrerequisiteScriptsAndCentOSInstallParse(t *testing.T) {
	bash, e := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		candidate := "C:/Program Files/Git/bin/bash.exe"
		if _, err := os.Stat(candidate); err == nil {
			bash, e = candidate, nil
		}
	}
	if e != nil {
		t.Skip("bash unavailable")
	}
	c, n := validDeployFixture()
	n[0].OS = "centos 7.9"
	n[0].Kernel = "5.10.240"
	n[0].Cgroup = "v1"
	c.DisableSELinux = true
	c.DisableFirewalld = true
	c.ProxyMode = "ipvs"
	scripts := []string{deployBootstrapScript(c), deployPrepareScript(c, n[0])}
	c.CustomSource = yumSourceFixture
	scripts = append(scripts, deployBootstrapScript(c), deployPrepareScript(c, n[0]))
	c.HA = true
	c.BuiltinLB = true
	c.APIServer = "192.0.2.100"
	c.APIPort = 8443
	c.RouterID = 68
	for i := 1; i < 3; i++ {
		node := n[0]
		node.HostID = uint(i + 1)
		n = append(n, node)
	}
	scripts = append(scripts, deployLBScript(c, n, 0))
	for _, script := range scripts {
		command := exec.Command(bash, "-n")
		command.Stdin = strings.NewReader(deployRootCommand(script))
		if out, e := command.CombinedOutput(); e != nil {
			t.Fatalf("shell syntax invalid: %s: %s", e, out)
		}
	}
	centos := expandedInstallScript(deployPrepareScript(c, n[0]))
	if strings.Contains(centos, "apt-get") {
		t.Fatal("CentOS installer used APT")
	}
	for _, required := range []string{"mirrors.aliyun.com/kubernetes-new/", "yum \"${ops_yum_args[@]}\"", centOSContainerdSHA256, centOSRuncSHA256, "sha256sum -c -", "SystemdCgroup = true", "containerd.service"} {
		if !strings.Contains(centos, required) {
			t.Fatalf("missing %q", required)
		}
	}
	if strings.Index(centos, centOSContainerdSHA256) > strings.Index(centos, "tar -xzf") {
		t.Fatal("archive extracted before verifying checksum")
	}
	c.CustomSource = ""
	bootstrap := expandedInstallScript(deployBootstrapScript(c))
	for _, forbidden := range []string{"kubeadm join", "kubeadm init", "swapoff", "setenforce", "systemctl stop firewalld", "--nogpgcheck", "sslverify=0", "trusted=yes", "AllowUnauthenticated=true"} {
		if strings.Contains(bootstrap, forbidden) {
			t.Fatalf("prerequisite preparation contains %q", forbidden)
		}
	}
	for _, required := range []string{"ubuntu:24.04:x86_64", "ubuntu:24.04:aarch64", "debian:13:x86_64", "mirrors.aliyun.com/ubuntu-ports", "mirrors.aliyun.com/debian-security", "mirrors.aliyun.com/centos-vault/7.9.2009", "flock -n 9", "保留现有时间同步服务"} {
		if !strings.Contains(bootstrap, required) {
			t.Fatalf("missing bootstrap path %q", required)
		}
	}
}

func TestDeployPhaseClaimsWorkWithMySQLChangedRows(t *testing.T) {
	active := true
	claims := 0
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		return nil, nil, errors.New("unexpected read")
	}, exec: func(q string, args []driver.NamedValue) (driver.Result, error) {
		if !active || !strings.Contains(q, "phase_sequence + 1") {
			return driver.RowsAffected(0), nil
		}
		claims++
		return driver.RowsAffected(1), nil
	}}
	s := &Service{db: systemDB(t, state)}
	for _, flag := range []string{"dependencies_prepared", "dependencies_prepared", "mutated", "mutated"} {
		if e := s.claimDeployPhase(1, flag); e != nil {
			t.Fatal(e)
		}
	}
	if claims != 4 {
		t.Fatal("repeated phases did not change the row")
	}
	active = false
	if e := s.claimDeployPhase(1, "mutated"); !errors.Is(e, errDeployStopped) {
		t.Fatal("lost ownership permitted mutation")
	}
}

func TestDeployCentOSProbeSupportsLegacyToolsWithoutRunningProbe(t *testing.T) {
	python, e := exec.LookPath("python3")
	if runtime.GOOS == "windows" || e != nil {
		python, e = exec.LookPath("python")
	}
	if e != nil {
		t.Skip("python unavailable")
	}
	harness := `import ast,sys,re,types,subprocess,ipaddress,gzip,io,json
import xml.etree.ElementTree as ET
source=sys.stdin.buffer.read().decode('utf-8')
try: tree=ast.parse(source,feature_version=(3,6))
except ValueError: tree=ast.parse(source,feature_version=(3,7))
assert 'text=True' not in source
def output(args,**kwargs):
    assert 'text' not in kwargs
    if '-j' in args: raise subprocess.CalledProcessError(1,args)
    if args[:3]==['ip','-o','-4']: return '1: lo inet 127.0.0.1/8 scope host lo\n2: ens33 inet 192.0.2.20/24 brd 192.0.2.255 scope global ens33\n'
    if args[:3]==['ip','-4','route']: return 'default via 192.0.2.1 dev ens33\n192.0.2.0/24 dev ens33 proto kernel scope link src 192.0.2.20\n'
    if args==['timedatectl','status']: return '      NTP synchronized: yes\n'
    raise AssertionError(args)
sp=types.SimpleNamespace(check_output=output,CalledProcessError=subprocess.CalledProcessError,DEVNULL=subprocess.DEVNULL)
def open_url(url,**kwargs):
    assert url.startswith('https://mirrors.aliyun.com/'),url
    if url.endswith('repomd.xml'): return io.BytesIO(b'<repomd xmlns="http://linux.duke.edu/metadata/repo"><data type="primary"><location href="repodata/test-primary.xml.gz"/></data></repomd>')
    if url.endswith('test-primary.xml.gz'): return io.BytesIO(gzip.compress(b'<metadata xmlns="http://linux.duke.edu/metadata/common"><package><name>kubeadm</name><arch>x86_64</arch><version ver="1.34.9"/></package></metadata>'))
    if url.endswith('Packages.gz'): return io.BytesIO(gzip.compress(b'Package: kubeadm\nVersion: 1.34.9-1.1\n'))
    raise AssertionError(url)
fakeos=types.SimpleNamespace(path=types.SimpleNamespace(exists=lambda p:False,isdir=lambda p:True))
ns=dict(subprocess=sp,ipaddress=ipaddress,re=re,gzip=gzip,json=json,ET=ET,urllib=types.SimpleNamespace(request=types.SimpleNamespace(urlopen=open_url)),os=fakeos,shutil=types.SimpleNamespace(which=lambda name:name if name=='timedatectl' else None))
functions=ast.Module(body=[node for node in tree.body if isinstance(node,ast.FunctionDef)],type_ignores=[])
exec(compile(functions,'probe-functions','exec'),ns)
interfaces,networks=ns['read_networks']()
assert interfaces[1]['ifname']=='ens33' and interfaces[1]['addr_info'][0]['local']=='192.0.2.20'
assert '192.0.2.0/24' in networks and '0.0.0.0/0' not in networks
assert ns['kernel_errors']('centos','3.10.0-1160.el7.x86_64','calico')
assert not ns['kernel_errors']('centos','5.10.240','calico')
assert not ns['kernel_errors']('centos','6.1.120','calico')
assert ns['time_synchronized']()
assert ns['package_available']('centos','1.34.9','x86_64')
assert not ns['package_available']('centos','1.34.8','x86_64')
assert ns['package_available']('ubuntu','1.34.9','x86_64')
block=next(node for node in tree.body if isinstance(node,ast.If) and ast.get_source_segment(source,node.test)=="cgroup=='v1'")
ns.update(cgroup='v1',p=dict(requireV2=False),errors=[])
exec(compile(ast.Module(body=[block],type_ignores=[]),'cgroup-check','exec'),ns)
assert not ns['errors']
ns['p']['requireV2']=True
exec(compile(ast.Module(body=[block],type_ignores=[]),'cgroup-check','exec'),ns)
assert ns['errors']
print('CentOS legacy iproute, Python 3.6 API, cgroup v1, kernel and Aliyun RPM/DEB metadata checks passed')`
	command := exec.Command(python, "-I", "-c", harness)
	command.Stdin = strings.NewReader(deployProbePython)
	if out, e := command.CombinedOutput(); e != nil {
		t.Fatalf("probe compatibility failed: %s\n%s", e, out)
	}
}

func TestNodeAddSecurityOptionsBindToPrecheck(t *testing.T) {
	c := nodeAddTestConfig()
	before := nodeAddInputHash(c)
	c.DisableSELinux = true
	if nodeAddInputHash(c) == before {
		t.Fatal("SELinux change did not invalidate input")
	}
	before = nodeAddInputHash(c)
	c.DisableFirewalld = true
	if nodeAddInputHash(c) == before {
		t.Fatal("firewall change did not invalidate input")
	}
}

func TestDeployContainerdSandboxConfigUsesSelectedRegistry(t *testing.T) {
	python, e := exec.LookPath("python3")
	if runtime.GOOS == "windows" || e != nil {
		python, e = exec.LookPath("python")
	}
	if e != nil {
		t.Skip("python unavailable")
	}
	c, _ := validDeployFixture()
	script := deploySandboxImageScript(c)
	_, body, ok := strings.Cut(script, "<<'OPS_ADMIN_SANDBOX'\n")
	if !ok {
		t.Fatal("missing sandbox configuration script")
	}
	body, _, _ = strings.Cut(body, "\nOPS_ADMIN_SANDBOX")
	harness := `import sys,io,json,re,contextlib
source=sys.stdin.buffer.read().decode('utf-8')
image='registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.10'
state={}
@contextlib.contextmanager
def open_config(path,mode='r'):
    assert path=='/etc/containerd/config.toml'
    f=io.StringIO('' if mode=='w' else state['input'])
    yield f
    if mode=='w': state['output']=f.getvalue()
for entry in ('version = 2\n[plugins."io.containerd.grpc.v1.cri"]\n  sandbox_image = "registry.k8s.io/pause:3.10"\n',"version = 3\n[plugins.'io.containerd.cri.v1.images'.pinned_images]\n  sandbox = 'registry.k8s.io/pause:3.10'\n"):
    state.clear(); state['input']=entry
    saved=sys.argv; sys.argv=['sandbox',image]
    try: exec(compile(source,'sandbox-config','exec'),dict(open=open_config))
    finally: sys.argv=saved
    assert image in state['output'] and 'registry.k8s.io/pause' not in state['output']
    assert state['output'].splitlines()[0]==entry.splitlines()[0]
`
	command := exec.Command(python, "-I", "-c", harness)
	command.Stdin = strings.NewReader(body)
	if out, e := command.CombinedOutput(); e != nil {
		t.Fatalf("sandbox configuration failed: %s\n%s", e, out)
	}
}
