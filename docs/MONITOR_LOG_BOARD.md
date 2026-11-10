# 日志大屏（VictoriaLogs）

入口：监控中心 → 日志大屏（`/monitor/log-board`），菜单位于「巡检大屏」之后。

## 目标

把 VictoriaLogs 里的日志按业务维度聚合成大屏，默认过滤 `kubernetes.container_name:"notice"`，
用于观察客户端错误上报的量级、类型和分布，不必逐条翻日志。

## 接口

`POST /api/v1/monitor/logboard/analysis`

| 字段 | 说明 |
| --- | --- |
| `datasourceId` | VictoriaLogs 数据源 ID（其他类型直接报错） |
| `query` | LogsQL 过滤条件，原样透传；留空等价 `*` |
| `startAt` / `endAt` | 毫秒时间戳 |
| `limit` | 取样上限，默认 5000，最大 20000 |

返回：`total` / `sampled` / `truncated` / `buckets[]` / `levels[]` / `namespaces[]` / `pods[]` /
`containers[]` / `errorTypes[]` / `patterns[]` / `facets[]` / `recent[]` / `podCount` / `nsCount`。

## 实现要点

- 总量与时间线走 `/select/logsql/hits`（精确）；明细走 `/select/logsql/query`，
  只有纯过滤条件时才追加 `| sort by (_time desc) | limit N`，带聚合管道的查询原样执行。
- **级别识别必须在服务端做**：日志正文带 Spring Boot 的 ANSI 颜色转义
  （`\x1b[31;1mERROR\x1b[m`），`ERROR` 会被切碎，LogsQL 的 `AND ERROR` 命中 0 条。
  所以先剥 ANSI，再用 `^\d{4}-… (TRACE|DEBUG|INFO|WARN|ERROR|FATAL) \d+ --- \[thread\] logger : content`
  取级别、线程、logger。
- 正文 `key=value, key=value` 用「下一个 key= 的起点」切分（值里允许逗号，如 userAgent），
  解析出的业务字段（`exceptionType` / `platform` / `appVersion` / `deviceModel` / `message` …）
  参与分布统计。
- `facets` 是动态字段：优先白名单业务字段，其余按覆盖度排序，只保留基数 2~40 的字段，
  跳过 `stackTrace`、设备唯一号这类高基数字段。
- `patterns` 用「异常类型 + 异常正文」做模板（无异常类型时退回首行），再把数字 / hex / UUID / IP
  折叠成 `<n>` / `<id>` / `<uuid>` / `<ip>`。单纯整行归一化会把所有上报并成一条，看不出哪类错误在涨。
- 下钻过滤用 `"字段=取值"` 短语（如 `"exceptionType=System.NullReferenceException"`）。
  这些维度是从正文解析出来的、**不是 VictoriaLogs 存储字段**，写 `exceptionType:xxx` 会命中 0 条。
- `/select/logsql/hits` 偶发超时会返回空结果，此时用样本自建时间线兜底，避免整块空白。

## 面板

指标卡（总量 / 错误 / 告警 / Pod / 样本 / 速率）、日志速率（堆叠柱，红色=错误）、
级别分布（环形）、错误类型 Top、日志模式 Top、动态字段分布（异常类型 / App 版本 / 日志类型 /
业务分类 / 设备型号 / 操作系统…点击即下钻）、最近错误日志（最近 50 条 ERROR/FATAL，
点击看完整正文与字段）。

「最近错误日志」只展示 `ERROR` / `FATAL`，不混入 `WARN` / `INFO`：值班排查要的是可执行的
错误信号，正常日志会把列表冲掉。条数上限为 50 条（`recentLimit`），`recentTotal` 是本次
取样窗口内的错误总数，被截断时面板副标题会写明「共 N 条，仅展示前 50 条」。

默认 24 小时、30 秒自动刷新（打开详情弹窗时暂停），数据源与过滤条件记在 localStorage。

## 已知边界

- 只支持 VictoriaLogs，Elasticsearch / SLS 会返回明确提示。
- 分布类面板基于取样（默认最新 5000 条），`total` 来自 hits 精确统计，两者可能有个位数差异。
- 聚合在服务端内存里完成，`limit` 上限 20000 即为单次分析的天花板。

## 公开分享（免登录）

入口：日志大屏工具栏「公开分享」。分享出去的地址是 `/public/log-board/<token>`，
客户端同事不需要账号、也不需要登录，打开就是同一块大屏；**页面里的数据源、LogsQL、
时间范围都能自己改**，所以换了数据源不需要重新生成链接。

### 分享保存什么

分享保存的是「打开时的默认视图」：默认数据源、默认 LogsQL、默认时间范围、刷新频率、
是否允许看详情、有效期。查看者可以在此基础上自己切换：

- 数据源：只能是 VictoriaLogs（服务端校验，其他类型直接报错），下拉框列出全部可用数据源；
- LogsQL：可自由修改，长度上限 2000 字符；
- 时间范围：5 分钟 ~ 7 天，超出会被夹回上限，取样固定 5000 条。

因此链接不是「冻结的快照」，而是「一块可交互的公开大屏」。这与内部大屏的区别只有：
不能下钻（避免把过滤条件拼成任意查询按钮）、不能保存、不能看分享入口。

| 字段 | 说明 |
| --- | --- |
| `name` / `description` | 大屏标题与说明；名称留空时按过滤条件自动生成 |
| `datasourceId` / `query` / `rangeSeconds` | 打开链接时的默认视图 |
| `refreshSeconds` | 公开页自动刷新间隔，15 秒 ~ 10 分钟 |
| `allowDetail` | 关闭后公开页不下发日志正文，仅保留首行摘要，并隐藏 Pod / 容器清单 |
| `expiresInSeconds` | `0` 为永久，否则到期后链接返回 410 |

### 接口

| 方法 | 路径 | 鉴权 |
| --- | --- | --- |
| `GET` | `/api/v1/monitor/log-board/shares` | 需登录 |
| `POST` | `/api/v1/monitor/log-board/shares/save` | 需登录 |
| `PUT` | `/api/v1/monitor/log-board/shares/status` | 需登录 |
| `POST` | `/api/v1/monitor/log-board/shares/token` | 需登录（重置链接） |
| `DELETE` | `/api/v1/monitor/log-board/shares/delete` | 需登录 |
| `GET` | `/api/v1/public/log-board/:token` | 免登录，返回标题、默认查询、数据源清单 |
| `POST` | `/api/v1/public/log-board/:token/data` | 免登录，body `{datasourceId, query, rangeSeconds}` |

状态码约定：`404` 链接不存在、`410` 已停用或已过期、`429` 访问过于频繁
（同一 token + IP 每分钟 40 次）。

### 实现要点

- 公开页走独立的 axios 实例：全局 `http` 对 401 的默认行为是跳登录页，
  公开页面不应该被会话状态影响。
- 同一 token + 数据源 + LogsQL + 时间范围在 3~8 秒内共用一次查询结果，
  投屏轮询或多个同事同时打开不会把 VictoriaLogs 打满。
- 公开查看者的选择记在 `localStorage`（`ops-admin-public-logboard:<token>`），
  下次打开沿用上次的条件；点「恢复默认条件」回到分享里的默认视图。
- 打开次数在页面加载（`GET` 元信息）时累加，自动刷新不重复计数。
- 分享记录保存创建人、打开次数、最近访问时间，管理端可随时停用、重置链接或删除。

## 已知边界

- 公开链接是「知道地址即可访问」，默认无密码；对外分发前建议设置有效期，
  或关闭「允许查看日志详情」。
- 公开页只读且不能下钻，面板几何与内部大屏保持一致。
