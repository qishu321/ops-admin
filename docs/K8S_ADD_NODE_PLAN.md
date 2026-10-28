# 节点管理添加节点

日期：2026-10-28。状态：已接入真实后端和普通本地页面；测试数据仅位于测试目录。

## 使用入口

更新代码并重启后端、更新前端构建（开发模式可刷新页面），在原有 `/containers/k8s/nodes` 页面选择集群，点击「添加节点」。无需模拟服务、额外端口或 nodeAddMode 标志。

流程：从 CMDB 选择服务器 → 配置节点名称、标签和污点 → SSH 环境预检 → 确认并添加 → 查看逐节点阶段、日志与 Ready 结果。

正常页面请求 `/api/v1/k8s/node-add/*`，连接本地已配置的真实后端。不存在浏览器内假数据、请求失败自动切换 mock 或模拟执行开关。测试服务不参与正常运行。

## 当前支持范围

- 首期添加 Worker，不改变现有控制平面拓扑，不重新初始化集群，也不重新安装 CNI。
- 支持标准 Kubernetes 1.31–1.34、containerd、IPv4 的 kubeadm 集群；包含手工登记但保留完整 kubeadm 引导配置及必要权限的集群。
- 新服务器支持现有安装模板中的 Ubuntu 22.04 / 24.04、Debian 12 / 13，x86_64 / aarch64、systemd 和 cgroup v2。节点必须无现有 Kubernetes、etcd 数据和正在运行的容器运行时。
- 从目标 API 读取版本、kubeadm-config、cluster-info、CA、公用入口及现有节点，沿用当前 Pod/Service 网络和镜像仓库。检测到 kube-proxy IPVS 时准备相应组件及内核模块。
- 从 CMDB 关联服务器、凭据与网关。OS、网卡和 machine-id 通过 SSH 实际核验，不让用户重复选择 OS。
- 集群 kubeconfig 需要启用 CA 校验，能读取相关集群配置、节点和引导角色绑定，能够创建与撤销 kube-system 内的短期 bootstrap Secret。缺少配置或权限时返回具体阻断原因。
- 本入口及真实执行接口限管理员；沿用现有全局只读和资源授权中间件。

## 预检与执行

预检异步执行并保存结果，只做探测和连通性验证，不安装、不 join。检查主机身份、OS/架构、CPU/内存/磁盘、干净状态、端口、网卡/IP、路由冲突、软件源、仓库与集群 API 可达性。

预检有效期五分钟，绑定输入配置、真实资产/凭据修订和目标集群身份。开始执行时重查这些信息，首次系统变更前再次核对集群；不复用过期检查。

部分 CNI 集群未在 kubeadm-config 中记录 Pod CIDR，此时不猜测网段、不阻断加入；预检及确认页明确提示未覆盖 Pod 网段冲突检查，需要核对现有 CNI 地址池。

确认后依次安装 kubelet/kubeadm/containerd、执行 kubeadm join、等待 Node Ready，并核对 machine-id、InternalIP 和 kubelet 版本。只操作用户选中的新节点。标签及污点写入加入配置；不修改既有节点的标签或污点。

每个加入步骤创建独立随机 bootstrap token，20 分钟到期，加入步骤结束后尝试撤销；token 不保存到业务模型、API 响应或日志中。加入配置以 0600 临时文件传递并在远端步骤退出时删除；不采用跳过 CA 校验的发现方式。

## 持久化与失败处理

新增表 `k8s_node_add_task` 保存配置、来源预检、幂等键、集群与资产快照关联；后端正常迁移流程负责创建表。执行状态、阶段、日志和主机互斥复用 `k8s_deploy_run/step/log/host_lock`。服务重启保留历史，心跳中断时核对执行权并标记失败或需要核对，不自动重新安装。

同一集群同一时间只执行一个预检/添加任务；主机与已有 K8s 部署任务共享占用锁。请求幂等键与完整输入绑定，每份成功预检只允许启动一个添加任务。

允许停止后续步骤，当前远程命令不会被当作已经回滚。变更前失败/未执行节点可以重新预检；变更后失败或状态不确定的节点保留占用锁并要求人工核对，不自动 reset、重装或提供盲目重试。成功节点保留成功状态并释放本次执行锁，后续通过真实集群节点信息阻止重复添加。

## 主要代码

- `backend/model/k8s_node_add.go`：任务与输入模型。
- `backend/service/k8s_node_add.go`：上下文、资产选择、校验、事务、持久化与任务查询。
- `backend/service/k8s_node_add_cluster.go`：集群配置、CA 身份和引导权限核对。
- `backend/service/k8s_node_add_runner.go`：真实 SSH 安装、bootstrap Secret、join、Ready 验证及结束处理。
- `backend/controller/k8s_node_add.go`、`backend/router/router.go`：正常后端接口。
- `web/src/views/assets/k8s/node-add/`：普通页面使用的向导、异步预检与进度。
- `web/tests/fixtures/`：仅供自动化/浏览器测试的测试替身，正常页面不导入。

## 验证边界

后端测试覆盖输入与保留标签、真实主机身份绑定、CA 公钥指纹、加入配置、临时 token 有效期与失败撤销、管理员边界、变更后锁保留和禁止盲目重试；所有 Kubernetes/SSH 测试均使用测试替身或只解析脚本。

执行后端 `go test ./...`、前端构建及关键流程浏览器检查。开发期间不启动连接现有数据库的后端，不在现有服务器上执行真实安装或 join。真实服务器上的安装耗时、网络及系统兼容性仍由本地实际验收确认，不将测试替身的成功声明为真实节点已加入。

参考：[kubeadm 添加 Linux Worker](https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/adding-linux-nodes/)、[kubeadm join](https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-join/)。
