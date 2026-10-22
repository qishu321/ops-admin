# [开源自荐] Ops Admin 两个月更新：新增中间件管理、JumpServer 资产同步，完善作业编排和监控告警

大家好，8 月底在 V2EX 分享过自己开发的开源运维平台 **Ops Admin**。距离上次发帖快两个月了，最近一直在结合日常运维中的需求补功能、修问题，来汇报一下进展。

- 项目地址：https://github.com/qishu321/ops-admin
- 项目网站：https://qishu321.github.io/ops-admin-website/
- 上次的介绍：https://www.v2ex.com/t/1238337

简单介绍一下：Ops Admin 基于 **Go + Vue 3**，主要面向个人运维和中小型运维团队，把主机、数据库、Kubernetes、批量执行、作业编排、监控告警和构建发布放到一个工作台里，也支持通过 SSH 网关访问内网资源。
- 项目截图
<img width="2536" height="1160" alt="Image" src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/01-platform-navigation.png" />
## 界面截图

下面是这次更新后的几张界面截图，每行两张。

<table>
  <tr>
    <td width="50%"><img src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/v2/17926648488217.png" alt="作业编排" width="100%" /></td>
    <td width="50%"><img src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/v2/17926646993809.png" alt="SSH 终端" width="100%" /></td>
  </tr>
  <tr>
    <td align="center"><sub>作业编排：条件分支与步骤配置</sub></td>
    <td align="center"><sub>SSH 终端：资产树与多标签会话</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/v2/17926648148673.png" alt="服务健康诊断" width="100%" /></td>
    <td width="50%"><img src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/v2/17926647672703.png" alt="Kubernetes 监控详情" width="100%" /></td>
  </tr>
  <tr>
    <td align="center"><sub>服务健康诊断：Arthas Java 诊断</sub></td>
    <td align="center"><sub>Kubernetes 监控详情：集群、节点与 Pod 指标</sub></td>
  </tr>
  <tr>
    <td colspan="2" align="center"><img src="https://github.com/qishu321/ops-admin/raw/main/docs/screenshots/v2/17926650933819.png" alt="日志查询" width="50%" /></td>
  </tr>
  <tr>
    <td colspan="2" align="center"><sub>日志查询：按命名空间与关键词检索</sub></td>
  </tr>
</table>

这次主要更新了以下几个部分。

## 1. 新增中间件管理

接入了 **RabbitMQ、Nacos、etcd、ZooKeeper 和 Kafka**，提供各自的管理工作台，认证信息复用平台的凭据管理。

- **RabbitMQ**：虚拟主机、队列、交换机、绑定关系、连接和消费者查看，以及常用管理操作。
- **Nacos**：适配 2.x、3.x，支持配置管理、服务发现、实例上下线和命名空间管理。
- **etcd**：按路径浏览 Key、查看和修改值、查看集群成员，支持 TLS 证书接入。
- **ZooKeeper**：节点树浏览、数据编辑、节点信息和 ACL 查看。
- **Kafka**：Topic 详情、分区与位点、消息浏览和发送、消费组、Topic 配置等。浏览消息时不加入消费组，也不提交消费位点。

这些功能主要覆盖日常排查和管理中常用的操作，还在继续完善。

## 2. 统一资产同步入口

新增了 **JumpServer 资产同步**，可以把资产及其分组关系同步到平台的主机和主机组中。

云主机同步也集中到了资产同步页面，减少来回切换入口、重复选择配置的操作。同步时复用凭据管理，预览中会过滤已经同步且没有变化的资产。

## 3. 完善作业编排和定时任务

重新调整了作业编排界面，并增加了**条件分支**。脚本执行、文件分发、人工确认和消息通知可以组合成作业，根据条件走不同流程。

同时优化了任务日志、脚本参数、定时任务和 HTTP 探针通知，修复重复通知等问题，让执行结果和失败原因更容易追踪。

## 4. Kubernetes 和终端体验

这部分做了不少日常使用上的改进：

- 完善集群、节点、Pod 和网络监控。
- 增加工作负载伸缩策略，优化镜像更新、环境变量查看等操作。
- 修复通过网关连接 Pod 终端的问题。
- SSH 和 Pod 终端支持 **5000 行历史回滚缓冲**，优化滚动和连接体验。
- 服务健康诊断页面切换标签后保留已有诊断状态，减少重复操作。

## 5. 调整权限管理

整理了之前比较分散的权限入口，围绕**用户、用户组、访问策略和权限模板**组织授权。

主机、Kubernetes、数据库、中间件等资源可以按用户或用户组授权，也补充了全局只读角色，修复了一些授权后页面需要刷新才能生效的问题。

另外新增了管理员维护的命令过滤规则，可分别作用于 SSH 终端和作业执行；Pod 终端不在过滤范围内。

## 6. 持续完善监控告警

新增了**告警规则导入、导出**，方便备份和迁移；监控大屏支持拖拽调整面板，并继续完善数据库及 Kubernetes 监控。

最近还集中处理了一个容易被忽略的问题：**指标查不到，不一定代表故障恢复。**例如 Pod 消失后，Ready 指标也可能消失。针对这类情况，补充了无数据处理和明确恢复条件，避免仅因为查询结果为空就发出恢复通知。


---

整体上，这两个月主要是在补齐中间件、资产同步和权限管理，同时把终端、作业执行、监控告警这些高频功能打磨得更顺手一些。

项目仍在持续迭代，欢迎感兴趣的朋友试用。遇到问题可以提 Issue；也想听听大家的意见：**你们日常运维中，最希望统一到一个平台里的操作是什么？**

如果觉得有用，也欢迎点个 Star，谢谢大家。
