export const APP_KEY = 'ops-admin-current-app'

export const appDefinitions = [
  {
    key: 'console',
    name: '控制台',
    labelKey: 'appConsole',
    icon: 'Monitor',
    defaultRoute: '/dashboard',
    menuSource: 'backend'
  },
  {
    key: 'assets',
    name: '资产管理',
    labelKey: 'appAssets',
    icon: 'Box',
    defaultRoute: '/assets/overview',
    menus: [
      {
        title: '资产概览',
        titleKey: 'assetsOverview',
        path: '/assets/overview',
        icon: 'DataBoard',
        children: []
      },
      {
        title: '环境模型',
        titleKey: 'opsEnvironments',
        path: '/assets/environments',
        icon: 'SetUp',
        children: []
      },
      {
        title: '终端登录',
        path: '/assets/terminal',
        icon: 'Platform',
        children: []
      },
      {
        title: '服务器管理',
        titleKey: 'serverManagement',
        path: '/assets/server',
        icon: 'Cpu',
        children: [
          {
            title: '主机管理',
            titleKey: 'hostManagement',
            path: '/assets/server/hosts',
            icon: 'Monitor',
            children: []
          },
          {
            title: '主机组管理',
            titleKey: 'hostGroupManagement',
            path: '/assets/server/groups',
            icon: 'FolderOpened',
            children: []
          },
          {
            title: '凭据管理',
            titleKey: 'credentialManagement',
            path: '/assets/server/credentials',
            icon: 'Key',
            children: []
          },
          {
            title: '云账号管理',
            titleKey: 'cloudAccountManagement',
            path: '/assets/server/cloud-accounts',
            icon: 'Cloudy',
            children: []
          }
        ]
      },
      {
        title: '数据库管理',
        titleKey: 'databaseManagement',
        path: '/assets/databases',
        icon: 'Coin',
        children: [
          {
            title: '数据库列表',
            titleKey: 'databaseList',
            path: '/assets/databases',
            icon: 'List',
            children: []
          },
          {
            title: 'DBMS 工作台',
            titleKey: 'databaseWorkbench',
            path: '/assets/databases/workbench',
            icon: 'EditPen',
            children: []
          },
          {
            title: '数据导入',
            titleKey: 'databaseImport',
            path: '/assets/databases/import',
            icon: 'Upload',
            children: []
          },
          {
            title: '备份管理',
            titleKey: 'databaseBackup',
            path: '/assets/databases/backups',
            icon: 'FolderOpened',
            children: []
          }
        ]
      },
      {
        title: '网关管理',
        titleKey: 'gatewayManagement',
        path: '/assets/gateways',
        icon: 'Switch',
        children: []
      },
    ]
  },
  {
    key: 'containers',
    name: '容器管理',
    labelKey: 'appContainers',
    icon: 'Box',
    defaultRoute: '/containers/k8s/clusters',
    menus: [
      {
        title: '服务管理',
        path: '/containers/services',
        icon: 'Grid',
        children: []
      },
      { title: '服务健康诊断', path: '/containers/services/health-diagnosis', icon: 'Monitor', children: [] },
      {
        title: 'Kubernetes 管理',
        titleKey: 'k8sManagement',
        path: '/containers/k8s',
        icon: 'Connection',
        children: [
          { title: '集群管理', titleKey: 'k8sClusters', path: '/containers/k8s/clusters', icon: 'FolderOpened', children: [] },
          { title: '集群概览', titleKey: 'k8sOverview', path: '/containers/k8s/overview', icon: 'DataAnalysis', children: [] },
          { title: '节点管理', titleKey: 'k8sNodes', path: '/containers/k8s/nodes', icon: 'Monitor', children: [] },
          { title: '命名空间', titleKey: 'k8sNamespaces', path: '/containers/k8s/namespaces', icon: 'Grid', children: [] },
          { title: '工作负载', titleKey: 'k8sWorkloads', path: '/containers/k8s/workloads', icon: 'SetUp', children: [] },
          { title: 'Pod 管理', titleKey: 'k8sPods', path: '/containers/k8s/pods', icon: 'Box', children: [] },
          { title: '服务', titleKey: 'k8sServices', path: '/containers/k8s/services', icon: 'Share', children: [] },
          { title: 'Ingress', titleKey: 'k8sIngresses', path: '/containers/k8s/ingresses', icon: 'Connection', children: [] },
          { title: '高级网络', titleKey: 'k8sAdvancedNetwork', path: '/containers/k8s/advanced-network', icon: 'Connection', children: [] },
          { title: '配置与存储', titleKey: 'k8sConfigStorage', path: '/containers/k8s/config-storage', icon: 'Files', children: [] },
          { title: '监控详情', titleKey: 'k8sMonitoringDetails', path: '/containers/k8s/monitoring', icon: 'Monitor', children: [] }
        ]
      }
    ]
  },
  {
    key: 'ops',
    name: '标准运维',
    labelKey: 'appOps',
    icon: 'Operation',
    defaultRoute: '/ops/quick-exec/command',
    menus: [
      {
        title: '脚本库',
        titleKey: 'opsScriptLibrary',
        path: '/ops/scripts/library',
        icon: 'Document',
        children: []
      },
      {
        title: '快速执行',
        titleKey: 'opsQuickExecute',
        path: '/ops/quick-exec',
        icon: 'Timer',
        children: [
          {
            title: '命令执行',
            titleKey: 'opsCommandExecute',
            path: '/ops/quick-exec/command',
            icon: 'CaretRight',
            children: []
          },
          {
            title: '脚本执行',
            titleKey: 'opsScriptExecute',
            path: '/ops/quick-exec/script',
            icon: 'EditPen',
            children: []
          },
          {
            title: '文件分发',
            titleKey: 'opsFileDispatch',
            path: '/ops/quick-exec/file-dispatch',
            icon: 'Files',
            children: []
          },
          {
            title: '快速执行历史',
            titleKey: 'opsExecutionHistory',
            path: '/ops/quick-exec/history',
            icon: 'DocumentCopy',
            children: []
          }
        ]
      },
      {
        title: '定时任务',
        titleKey: 'opsSchedule',
        path: '/ops/schedule',
        icon: 'Clock',
        children: [
          {
            title: '任务列表',
            titleKey: 'opsScheduleTasks',
            path: '/ops/schedule/tasks',
            icon: 'List',
            children: []
          },
          {
            title: '任务日志',
            titleKey: 'opsScheduleLogs',
            path: '/ops/schedule/logs',
            icon: 'Document',
            children: []
          },
          {
            title: '任务模板',
            titleKey: 'opsScheduleTemplates',
            path: '/ops/schedule/templates',
            icon: 'Tickets',
            children: []
          }
        ]
      },
      {
        title: '作业',
        titleKey: 'opsJobs',
        path: '/ops/jobs',
        icon: 'Share',
        children: [
          {
            title: '作业编排',
            titleKey: 'opsJobDesigner',
            path: '/ops/jobs/designer',
            icon: 'Grid',
            children: []
          },
          {
            title: '作业列表',
            titleKey: 'opsJobList',
            path: '/ops/jobs/list',
            icon: 'List',
            children: []
          },
          {
            title: '人工确认',
            titleKey: 'opsJobApprovals',
            path: '/ops/jobs/approvals',
            icon: 'Bell',
            children: []
          },
          {
            title: '作业历史',
            titleKey: 'opsJobHistory',
            path: '/ops/jobs/history',
            icon: 'Document',
            children: []
          },
          {
            title: '作业模板',
            titleKey: 'opsJobTemplates',
            path: '/ops/jobs/templates',
            icon: 'Tickets',
            children: []
          }
        ]
      }
    ]
  },
  {
    key: 'applications',
    name: '应用中心',
    labelKey: 'appApplications',
    icon: 'Box',
    defaultRoute: '/applications/projects',
    menus: [
      {
        title: '应用管理',
        titleKey: 'appProjectList',
        path: '/applications/projects',
        icon: 'Tickets',
        children: []
      },
      {
        title: '构建与部署',
        titleKey: 'appBuildDeploy',
        path: '/applications/build-tasks',
        icon: 'SetUp',
        children: [
          {
            title: '构建任务',
            titleKey: 'appBuildTasks',
            path: '/applications/build-tasks',
            icon: 'Operation',
            children: []
          },
          {
            title: '构建历史',
            titleKey: 'appBuildHistory',
            path: '/applications/build-history',
            icon: 'Document',
            children: []
          },
        ]
      },
      {
        title: '镜像仓库',
        path: '/applications/image-registries',
        icon: 'Box',
        children: []
      },
      {
        title: 'CI/CD 流水线',
        titleKey: 'appPipelines',
        path: '/applications/pipelines',
        icon: 'Share',
        children: []
      }
    ]
  },
  {
    key: 'notify',
    name: '消息通知',
    labelKey: 'appNotify',
    icon: 'Bell',
    defaultRoute: '/notify/rules',
    menus: [
      {
        title: '通知规则',
        titleKey: 'notifyRules',
        path: '/notify/rules',
        icon: 'Operation',
        children: []
      },
      {
        title: '消息模板',
        titleKey: 'notifyTemplates',
        path: '/notify/templates',
        icon: 'Document',
        children: []
      },
      {
        title: '通知媒介',
        titleKey: 'notifyChannels',
        path: '/notify/channels',
        icon: 'Connection',
        children: []
      },
      {
        title: '发送日志',
        titleKey: 'notifySendLogs',
        path: '/notify/send-logs',
        icon: 'Tickets',
        children: []
      }
    ]
  },
  {
    key: 'integration',
    name: '集成中心',
    labelKey: 'appIntegration',
    icon: 'Grid',
    defaultRoute: '/integration/navigation',
    menus: [
      {
        title: '导航管理',
        titleKey: 'integrationNavigation',
        path: '/integration/navigation',
        icon: 'Grid',
        children: []
      },
      {
        title: 'AI 助手',
        titleKey: 'integrationAI',
        path: '/integration/ai/chat',
        icon: 'ChatLineRound',
        children: [
          { title: '智能对话', titleKey: 'integrationAIChat', path: '/integration/ai/chat', icon: 'ChatDotRound', children: [] },
          { title: '会话管理', titleKey: 'integrationAIConversations', path: '/integration/ai/conversations', icon: 'Clock', children: [] },
          { title: '模型管理', titleKey: 'integrationAIModels', path: '/integration/ai/models', icon: 'Cpu', children: [] },
          { title: '知识库管理', titleKey: 'integrationAIKnowledgeBase', path: '/integration/ai/knowledge-base', icon: 'Document', children: [] },
          { title: '工具集', titleKey: 'integrationAITools', path: '/integration/ai/tools', icon: 'Operation', children: [] }
        ]
      },
      {
        title: '云费用分析',
        titleKey: 'integrationCloudCost',
        path: '/integration/finops/dashboard',
        icon: 'Coin',
        children: [
          { title: '费用看板', titleKey: 'integrationCostDashboard', path: '/integration/finops/dashboard', icon: 'DataAnalysis', children: [] },
          { title: '云账号', titleKey: 'integrationCloudAccounts', path: '/integration/finops/accounts', icon: 'CreditCard', children: [] },
          { title: '费用拆分', titleKey: 'integrationCostBreakdown', path: '/integration/finops/breakdown', icon: 'PieChart', children: [] },
          { title: '优化建议', titleKey: 'integrationCostRecommendations', path: '/integration/finops/recommendations', icon: 'Opportunity', children: [] },
          { title: '资源拆分', titleKey: 'integrationCostResources', path: '/integration/finops/resources', icon: 'Box', children: [] },
          { title: '账单同步', titleKey: 'integrationCostSync', path: '/integration/finops/sync', icon: 'Refresh', children: [] }
        ]
      }
    ]
  },
  {
    key: 'monitor',
    name: '监控中心',
    labelKey: 'appMonitor',
    icon: 'Histogram',
    defaultRoute: '/monitor/overview',
    menus: [
      {
        title: '监控概览',
        titleKey: 'monitorOverview',
        path: '/monitor/overview',
        icon: 'TrendCharts',
        children: []
      },
      {
        title: '智能大屏',
        path: '/monitor/command-center',
        icon: 'DataBoard',
        children: []
      },
      {
        title: '数据源管理',
        titleKey: 'monitorDatasources',
        path: '/monitor/datasources',
        icon: 'Connection',
        children: []
      },
      {
        title: '即时查询',
        titleKey: 'monitorQuery',
        path: '/monitor/query',
        icon: 'Search',
        children: []
      },
      {
        title: '日志查询',
        titleKey: 'monitorLogs',
        path: '/monitor/logs',
        icon: 'Document',
        children: []
      },
      {
        title: '链路追踪',
        titleKey: 'monitorTraces',
        path: '/monitor/traces',
        icon: 'Share',
        children: []
      },
      {
        title: '告警管理',
        path: '/monitor/alert-management',
        icon: 'Bell',
        children: [
          { title: '告警模板', path: '/monitor/alert-templates', icon: 'CollectionTag', children: [] },
          { title: '告警规则', titleKey: 'monitorAlertRules', path: '/monitor/alert-rules', icon: 'Bell', children: [] },
          { title: '告警事件', titleKey: 'monitorAlertEvents', path: '/monitor/alert-events', icon: 'Warning', children: [] },
          { title: '告警屏蔽', titleKey: 'monitorSilences', path: '/monitor/silences', icon: 'MuteNotification', children: [] },
          { title: '聚合收敛', titleKey: 'monitorAggregations', path: '/monitor/aggregations', icon: 'Filter', children: [] }
        ]
      },
      {
        title: '监控大屏',
        titleKey: 'monitorDashboards',
        path: '/monitor/dashboards',
        icon: 'PieChart',
        children: []
      },
      {
        title: '巡检大屏',
        titleKey: 'monitorInspections',
        path: '/monitor/inspections',
        icon: 'Tickets',
        children: []
      }
    ]
  },
  {
    key: 'domains',
    name: '域名管理',
    labelKey: 'appDomains',
    icon: 'Connection',
    defaultRoute: '/domains/public',
    menus: [
      {
        title: '公网域名',
        path: '/domains/public',
        icon: 'Position',
        children: [
          { title: '域名列表', path: '/domains/public', icon: 'List', children: [] },
          { title: 'DNS 账号', path: '/domains/public/accounts', icon: 'Key', children: [] },
          { title: 'SSL 证书', path: '/domains/public/certificates', icon: 'Lock', children: [] }
        ]
      },
      {
        title: '内网域名',
        path: '/domains/internal',
        icon: 'OfficeBuilding',
        children: [
          { title: 'Zone 管理', path: '/domains/internal', icon: 'Collection', children: [] },
          { title: 'DNS 设置', path: '/domains/internal/settings', icon: 'Setting', children: [] }
        ]
      },
      {
        title: '诊断与审计',
        path: '/domains/query-test',
        icon: 'DataAnalysis',
        children: [
          { title: '解析测试', path: '/domains/query-test', icon: 'Search', children: [] },
          { title: '操作审计', path: '/domains/audit', icon: 'Memo', children: [] }
        ]
      }
    ]
  }
]

export function getAppByKey(key) {
  return appDefinitions.find((item) => item.key === key) || appDefinitions[0]
}

export function getAppByRoute(path) {
  if (path.startsWith('/assets')) {
    return getAppByKey('assets')
  }
  if (path.startsWith('/containers')) {
    return getAppByKey('containers')
  }
  if (path.startsWith('/ops')) {
    return getAppByKey('ops')
  }
  if (path.startsWith('/applications')) {
    return getAppByKey('applications')
  }
  if (path.startsWith('/notify')) {
    return getAppByKey('notify')
  }
  if (path.startsWith('/monitor')) {
    return getAppByKey('monitor')
  }
  if (path.startsWith('/integration')) {
    return getAppByKey('integration')
  }
  if (path.startsWith('/domains')) {
    return getAppByKey('domains')
  }
  return getAppByKey('console')
}

export function setCurrentApp(key) {
  localStorage.setItem(APP_KEY, key)
}

export function getCurrentApp() {
  return localStorage.getItem(APP_KEY) || 'console'
}

