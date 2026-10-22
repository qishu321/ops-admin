# 中间件管理

位置：资产管理 → 中间件管理，在数据库管理与凭据管理之间。

## 接入

管理员新增数据源并测试连接。认证复用凭据管理中已启用的凭据，只保存凭据 ID，不在页面返回密码或证书内容。修改凭据后，下次操作读取最新凭据。

凭据管理是资产管理下与中间件管理同级的一级入口，主机登录、资产同步与 TLS 加密连接的凭据统一在这里维护。TLS 凭据保存 CA 证书、客户端证书与客户端私钥（PEM），支持直接粘贴或从本地选择文件，内容不会回显，仅在同一凭据编辑时留空保留原值。

| 类型 | 地址与认证 | 工作台 |
| --- | --- | --- |
| RabbitMQ | Management HTTP API，默认 `http://host:15672`，用户名密码 | 独立工作台：虚拟主机侧栏、实例概览、队列、交换机、连接、消费者；创建/删除队列与交换机、批量删除队列、清空队列、队列/交换机详情与绑定、关闭连接 |
| Nacos 2.x | `http://host:8848/nacos`；使用 2.x 保留的 v1 管理接口，兼容早期 2.x | 独立工作台：配置管理、服务发现、命名空间；配置查询/新建/编辑/删除/批量删除、实例上下线与移除、命名空间创建与删除 |
| Nacos 3.x | 控制台 API 地址，默认 `http://host:8080`，自定义控制台根路径需包含；使用 v3 console API，不依赖 v1/v2 兼容开关 | 同上；默认命名空间为 public，配置与服务列表使用 v3 分页接口 |
| etcd | v3 HTTP gateway，默认 `http://host:2379`；支持用户名密码与 TLS 证书：选择「TLS 证书（mTLS）」后绑定凭据管理中的 TLS 凭据，地址以 `https://` 保存，CA 校验证书链、客户端证书完成双向认证；也可只开启传输加密配合用户名密码 | 前缀浏览、精确读取/写入/删除 Key、集群成员 |
| ZooKeeper | 一个或多个 `host:2181`，每行一个；Digest 用户名密码 | 节点树浏览、节点数据与节点信息、ACL 查看、创建、修改、删除；修改/删除需要当前 version；新节点继承父节点 ACL，不递归删除 |
| Kafka | 一个或多个 `host:9092`，每行一个；无认证、SASL/PLAIN、SCRAM-SHA-256/512，可开启 TLS | Broker、Topic/分区、所有 Broker 的消费组、创建/删除 Topic、清空 Topic 消息 |

HTTP 数据源填写单个服务或负载均衡地址；Kafka、ZooKeeper 可填写多个节点。HTTPS/TLS 使用系统受信任证书，不跳过证书校验。ZooKeeper 当前不支持 TLS 客户端证书接入。Kafka 的 advertised.listeners 地址必须对后台可达。

RabbitMQ 需要启用 management 插件。消费者不能通过 HTTP API 单独删除，可关闭对应连接，连接上所有通道及消费者会同时断开。

Nacos、RabbitMQ 等远端账号自身也需具备对应 API 权限。Nacos 3.x 的控制台端口与服务端 8848 不同，不能直接混用。每次操作重新登录获取 token，不向浏览器返回 token。

## ZooKeeper 节点浏览

从数据源列表点击名称或“工作台”进入独立的节点浏览页 `/assets/middleware/zookeeper/explorer?sourceId=<id>`，也可附加 `path` 参数直接定位节点。

- 左侧为按需加载的节点树，展开时才请求下一级子节点；`children` 带 `detail=1` 时额外返回子节点摘要（是否有下级、数据大小、修改时间），用 `Exists` 只取 Stat，不传输节点数据，单次最多 500 个子节点并给出截断提示。
- 单击目录节点即选中并展开下一级，收起需要点左侧箭头；从深链或面包屑跳转到某个节点时，会逐级展开祖先并滚动到该节点。
- 顶部搜索框筛选已加载节点；输入以 `/` 开头的完整路径并回车可直接跳转到该节点。
- 右侧展示路径面包屑、修订/版本/租约/大小、节点数据与 `节点信息`（zxid、ctime、mtime、version、cversion、aversion、dataLength、numChildren、ephemeralOwner）。数据为合法 JSON 时可切换“格式化”，只影响展示，不改变节点内容。
- 编辑与删除沿用读取时的 version 做乐观锁，期间被其他客户端修改会保存失败；删除含子节点的节点会被拒绝，需要先清理下级。编辑、创建需要写入授权，删除需要删除授权。

## Kafka Topic 浏览

在 Kafka 工作台的 Topic 列表点击 Topic 名称进入 `/assets/middleware/kafka/topic?sourceId=<id>&name=<topic>`，页内分为概览、消息、消费者、配置、统计五个标签。

- “消息”按分区直接 Fetch 读取，不加入消费组、不提交位点；支持最新、最早、指定 Offset、指定时间四种定位方式，单页最多 200 条、4 MiB，单条超过 1 MiB 会提示改用专用客户端。
- 单击任意消息行（或行尾“查看 / 收起”）就地展开详情，不再弹窗：展开后整行浅蓝高亮并在首列带主色标记，详情卡片自带标题栏显示分区与 Offset，可点“收起”或再次点击该行关闭。左侧切换 Key / Value / Headers，Value 可一键格式化 JSON 并复制，右侧列出记录时间（CreateTime）、分区、Offset、Key/Value 的编码与字节数。
- Tombstone（Value 为 null 的删除标记）在列表中单独标记，展开后 Value 显示为 `null`；非 UTF-8 内容以 Base64 展示并标注编码。
- 翻页按 `nextOffset` 前进，`hasMore` 表示分区末尾之后仍有数据，“上一页”依赖浏览历史游标回退。
- 标题栏“发送消息”右侧的“清空消息”会删除该 Topic 的全部消息，需要删除授权并二次确认。Kafka 没有原地清空接口，实现方式是先删除 Topic，再按原分区数与副本因子重建，并逐项写回原有的 Topic 级配置；重建期间生产与消费会短暂中断，消费组已提交位点失效，消息不可恢复。内部 Topic 拒绝清空；删除后 30 秒内 Broker 未完成清理会直接报错，不会继续重建。

Kafka 工作台的“管理操作”只保留创建 Topic、删除 Topic；消费组在“消费组”标签页查看，点行内“详情”即可展开分区位点与客户端信息，不再单列菜单项。

## RabbitMQ 工作台

从数据源列表进入 `/assets/middleware/rabbitmq/explorer?sourceId=<id>`，可通过 `vhost`、`tab` 参数直接定位。

- 左侧“虚拟主机”来自 `/api/vhosts`，支持按名称搜索；顶部“全部虚拟主机”会跨主机查询队列、交换机与消费者，列表里额外标注每个资源所属的 vhost。
- “实例概览”展示 RabbitMQ/Management 版本、集群与节点、队列/交换机/连接/消费者数量、队列消息总量、投递与发布速率、磁盘与内存，并列出各虚拟主机的消息、待消费、未确认数量，点击行可跳转到该主机的队列列表。
- “队列”列出消息总数、待消费/未确认、消费者数与状态（运行中/空闲/限流/停止）。列表顶部可按队列名称过滤，`创建队列` 支持持久化、自动删除与 JSON 扩展参数（可选择 Classic/Quorum/Stream 队列类型，会自动合并 `x-queue-type`）；勾选多行可“批量删除”，逐个删除并汇总失败项。
- 点击队列名称打开详情抽屉：展示类型、vhost、持久化、自动删除、状态、消息/待消费/未确认、消费者、内存、节点、策略、投递/确认速率；`绑定关系` 列出源交换机与 Routing key，`扩展参数` 展示声明参数，`消费者` 列出 consumer tag、连接与预取数量。底部可直接清空消息或删除队列。
- “交换机”支持创建与删除，点击名称查看类型、持久化、内部交换机、消息流入、扩展参数与绑定关系；默认交换机没有名称，只展示概览，不提供删除。
- “连接”展示用户、客户端地址、vhost、状态与通道数，可强制关闭连接；“消费者”展示队列、连接、通道、消息确认方式与预取数量。消费者随连接/通道生命周期管理，需关闭对应连接。
- `queue.put` / `exchange.put` 的 `arguments` 必须是 JSON 对象；队列/交换机声明幂等，同名同参数不会重建，参数不一致由服务端拒绝。

## Nacos 工作台

从数据源列表进入 `/assets/middleware/nacos/explorer?sourceId=<id>`，支持 `namespaceId`、`tab`、`dataId`、`group` 深链参数。

- 顶部工具栏常驻命名空间选择与查询条件；“配置管理”按 Data ID / Group 查询并分页（10/20/50 条），表格展示格式、应用与更新时间，`新建配置`、`编辑`、`删除`、`批量删除` 按写入/删除授权控制。
- 配置详情抽屉读取原始内容（Nacos 3.x 会从详情对象中提取 `content`），支持复制与 JSON 格式化；编辑弹窗可修改格式、应用名、描述与内容，发布后客户端会收到变更通知。
- “服务发现”通过 Nacos 服务目录返回实例数与健康实例数，点击“查看实例”打开抽屉，列出 IP、端口、集群、健康、启用、权重、临时实例；可通过开关上线/下线实例（更新 `enabled` 与权重），或移除实例。2.x 使用 v1 目录接口，3.x 使用 v3 console 分页接口。
- “命名空间”列出 ID、名称、描述与配置数量，支持创建与删除；public 为默认命名空间不可删除。2.x 的 public 使用空 tenant，3.x 使用 `public`，页面统一展示为 public，请求时按版本转换。
- 页面只调用白名单操作：`configs`、`config.get/put/delete`、`services`、`instances`、`instance.put/delete`、`namespaces`、`namespace.put/delete`，不会把任意参数透传给远端。

## 授权与确认

访问策略新增独立资源类型“中间件”，按数据源实例授权给用户或用户组，也支持用户组绑定策略/权限模板。操作包括：查看、查询、写入/变更、删除、全部资源操作。未授权数据源在 SQL 查询阶段过滤，查询/变更接口再校验目标和操作，不能在查询接口提交变更操作。全局只读仍限制写入与删除。

数据源配置维护与测试只允许平台管理员；授权操作不会授予查看原始连接配置或凭据的权限。所有变更需页面确认及后端 confirmed 参数。删除数据源只移除连接记录，不删除远端资源。

接口前缀 `/api/v1/asset/middleware`：GET list；POST save、test、query、write、remove；DELETE delete。`query/write/remove` 只接收白名单操作，不提供任意远端 URL 或原始命令代理。

## 限制与验证

远端请求限时 12 秒、HTTP 响应上限 8 MiB；HTTP 不跟随重定向，避免认证信息被转发。Nacos 配置与服务每页默认 10 条，可选 10/20/50（后端上限 500）；etcd 前缀每次最多 200 个 Key，使用更具体的前缀缩小范围。二进制 Key/Value 以文本展示，当前不提供任意二进制编辑。Nacos 3.x 实例上下线沿用 console 实例接口，若远端版本调整了表单字段，需要按实际版本联调。

新增表 `asset_middleware`，随后端启动自动迁移；同时更新前端和后端，后端重启时初始化菜单与权限项。

验证包括 HTTP 假服务的 Nacos 2/3 登录、路径与参数映射、RabbitMQ 路径转义、etcd 精确删除/编码、重定向防护，以及资源授权正反例；前端数据解析测试与本地模拟 API 浏览器检查。没有连接用户的真实中间件集群，原生 Kafka/ZooKeeper 的线上认证和操作需在实际环境联调。

参考：[RabbitMQ HTTP API](https://www.rabbitmq.com/docs/http-api-reference)、[Nacos 3.x Console API](https://nacos.io/en/docs/v3.0/manual/admin/console-api/)、[Nacos 2.x API](https://nacos.io/en/docs/v2/guide/user/open-api/)、[etcd v3 gateway](https://etcd.io/docs/v3.6/dev-guide/api_grpc_gateway/)。
