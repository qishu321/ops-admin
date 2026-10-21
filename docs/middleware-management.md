# 中间件管理

位置：资产管理 → 中间件管理，在数据库管理与网关管理之间。

## 接入

管理员新增数据源并测试连接。认证复用凭据管理中已启用的用户名密码凭据，只保存凭据 ID，不在页面返回密码。修改凭据后，下次操作读取最新凭据。

| 类型 | 地址与认证 | 工作台 |
| --- | --- | --- |
| RabbitMQ | Management HTTP API，默认 `http://host:15672`，用户名密码 | 队列、交换机、连接、消费者、通道、概览；创建/删除队列与交换机、队列绑定、清空队列、关闭连接 |
| Nacos 2.x | `http://host:8848/nacos`；使用 2.x 保留的 v1 管理接口，兼容早期 2.x | 配置列表/读取/发布/删除、服务列表/实例更新与移除、命名空间创建与删除 |
| Nacos 3.x | 控制台 API 地址，默认 `http://host:8080`，自定义控制台根路径需包含；使用 v3 console API，不依赖 v1/v2 兼容开关 | 同上；默认命名空间为 public |
| etcd | v3 HTTP gateway，默认 `http://host:2379`；可配置用户名密码 | 前缀浏览、精确读取/写入/删除 Key、集群成员 |
| ZooKeeper | 一个或多个 `host:2181`，每行一个；Digest 用户名密码 | 节点层级、数据与 ACL 查看、创建、修改、删除；修改/删除需要当前 version；新节点继承父节点 ACL，不递归删除 |
| Kafka | 一个或多个 `host:9092`，每行一个；无认证、SASL/PLAIN、SCRAM-SHA-256/512，可开启 TLS | Broker、Topic/分区、所有 Broker 的消费组、消费组详情、创建/删除 Topic、删除消费组 |

HTTP 数据源填写单个服务或负载均衡地址；Kafka、ZooKeeper 可填写多个节点。HTTPS/TLS 使用系统受信任证书，不跳过证书校验。ZooKeeper 当前不支持 TLS 客户端证书接入。Kafka 的 advertised.listeners 地址必须对后台可达。

RabbitMQ 需要启用 management 插件。消费者不能通过 HTTP API 单独删除，可关闭对应连接，连接上所有通道及消费者会同时断开。

Nacos、RabbitMQ 等远端账号自身也需具备对应 API 权限。Nacos 3.x 的控制台端口与服务端 8848 不同，不能直接混用。每次操作重新登录获取 token，不向浏览器返回 token。

## 授权与确认

访问策略新增独立资源类型“中间件”，按数据源实例授权给用户或用户组，也支持用户组绑定策略/权限模板。操作包括：查看、查询、写入/变更、删除、全部资源操作。未授权数据源在 SQL 查询阶段过滤，查询/变更接口再校验目标和操作，不能在查询接口提交变更操作。全局只读仍限制写入与删除。

数据源配置维护与测试只允许平台管理员；授权操作不会授予查看原始连接配置或凭据的权限。所有变更需页面确认及后端 confirmed 参数。删除数据源只移除连接记录，不删除远端资源。

接口前缀 `/api/v1/asset/middleware`：GET list；POST save、test、query、write、remove；DELETE delete。`query/write/remove` 只接收白名单操作，不提供任意远端 URL 或原始命令代理。

## 限制与验证

远端请求限时 12 秒、HTTP 响应上限 8 MiB；HTTP 不跟随重定向，避免认证信息被转发。Nacos 配置/服务每页 100 条；etcd 前缀每次最多 200 个 Key，使用更具体的前缀缩小范围。二进制 Key/Value 以文本展示，当前不提供任意二进制编辑。

新增表 `asset_middleware`，随后端启动自动迁移；同时更新前端和后端，后端重启时初始化菜单与权限项。

验证包括 HTTP 假服务的 Nacos 2/3 登录、路径与参数映射、RabbitMQ 路径转义、etcd 精确删除/编码、重定向防护，以及资源授权正反例；前端数据解析测试与本地模拟 API 浏览器检查。没有连接用户的真实中间件集群，原生 Kafka/ZooKeeper 的线上认证和操作需在实际环境联调。

参考：[RabbitMQ HTTP API](https://www.rabbitmq.com/docs/http-api-reference)、[Nacos 3.x Console API](https://nacos.io/en/docs/v3.0/manual/admin/console-api/)、[Nacos 2.x API](https://nacos.io/en/docs/v2/guide/user/open-api/)、[etcd v3 gateway](https://etcd.io/docs/v3.6/dev-guide/api_grpc_gateway/)。
