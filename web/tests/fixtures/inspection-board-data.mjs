export const boardSources = [
  { id: 1, name: '上海 · 测试集群', type: 'prometheus', isDefault: true, status: 1 },
  { id: 2, name: '压测61环境', type: 'victoriametrics', status: 1 },
  { id: 3, name: 'test-51', type: 'prometheus', status: 1 },
]
const definitions = [
  ['主机总数', '台', '资源概况', 'count(up)', ['2台', '9台', '12台']],
  ['在线主机', '台', '可用性', 'sum(up)', ['2台', '9台', '12台']],
  ['离线主机', '台', '可用性', 'count(up == 0)', ['无数据', '无数据', '0台']],
  ['CPU 使用率', '%', '主机资源', 'avg(cpu_usage_percent)', ['18.4%', '28.1%', '34.8%']],
  ['内存使用率', '%', '主机资源', 'avg(memory_usage_percent)', ['42.6%', '62.1%', '51.4%']],
  ['磁盘使用率', '%', '主机资源', 'avg(disk_usage_percent)', ['61.2%', '54.3%', '39.1%']],
  ['磁盘读取', 'B/s', '磁盘与网络', 'sum(rate(node_disk_read_bytes_total[5m]))', ['1.28 MiB/s', '3.12 MiB/s', '832 KiB/s']],
  ['磁盘写入', 'B/s', '磁盘与网络', 'sum(rate(node_disk_written_bytes_total[5m]))', ['480 KiB/s', '1.20 MiB/s', '256 KiB/s']],
  ['网络接收', 'B/s', '磁盘与网络', 'sum(rate(node_network_receive_bytes_total[5m]))', ['3.20 MiB/s', '8.40 MiB/s', '2.12 MiB/s']],
  ['网络发送', 'B/s', '磁盘与网络', 'sum(rate(node_network_transmit_bytes_total[5m]))', ['1.15 MiB/s', '6.20 MiB/s', '980 KiB/s']],
  ['1 分钟负载', '', '系统负载', 'avg(node_load1)', ['0.21', '0.85', '0.46']],
  ['5 分钟负载', '', '系统负载', 'avg(node_load5)', ['0.18', '0.72', '0.39']],
  ['15 分钟负载', '', '系统负载', 'avg(node_load15)', ['0.16', '0.64', '0.35']],
  ['运行时长', '小时', '系统状态', 'avg(time() - node_boot_time_seconds) / 3600', ['720小时', '480小时', '960小时']],
  ['TCP 连接数', '个', '系统状态', 'sum(node_netstat_Tcp_CurrEstab)', ['156个', '428个', '209个']],
  ['网络丢包率', '%', '系统状态', 'sum(rate(node_network_receive_drop_total[5m]))', ['0%', '0%', '0%']],
  ['主机重启次数', '次', '系统状态', 'sum(changes(node_boot_time_seconds[1h]))', ['0次', '0次', '0次']],
]
export const boardPanels = definitions.map(([title, unit, category, promql], index) => ({
  id: 201 + index, dashboardId: 10, datasourceId: 1, title, unit, category, promql, sort: index, status: 1,
  chartType: 'stat', inspectionKind: [2, 3, 4, 5, 15, 16].includes(index) ? 'rule' : 'context',
  reducer: index < 3 ? 'last' : 'max', operator: 'gte', warningValue: index === 2 ? 0.5 : 80, criticalValue: index === 2 ? 0.5 : 90, noDataState: 'warning',
}))
export function createBoardReport(payload, failure = false) {
  const ids = payload.datasourceIds
  const results = ids.flatMap(id => boardPanels.map((panel, index) => {
    const noData = index === 2 && id !== 3
    const failed = failure && id === 3 && index === 3
    return {
      ...panel, id: id * 1000 + panel.id, panelId: panel.id, runId: 88, datasourceId: id,
      datasourceName: boardSources.find(source => source.id === id).name,
      status: failed ? 'danger' : noData ? 'warning' : 'healthy',
      valueText: failed ? '--' : definitions[index][4][id - 1], seriesCount: failed || noData ? 0 : 1,
      error: failed ? '模拟查询超时：用于验证错误详情，不查询真实监控系统。' : '',
    }
  }))
  return { run: {
    id: 88, dashboardId: 10, dashboardName: '生产环境基础巡检', datasourceId: ids[0], datasourceName: boardSources.find(source => source.id === ids[0]).name,
    datasourceIds: ids, datasourceNames: ids.map(id => boardSources.find(source => source.id === id).name),
    startAt: new Date(payload.startAt * 1000).toISOString(), endAt: new Date(payload.endAt * 1000).toISOString(),
    completedAt: new Date().toISOString(), durationMs: 276, totalCount: results.length,
    healthyCount: results.filter(row => row.status === 'healthy').length,
    warningCount: results.filter(row => row.status === 'warning').length,
    dangerCount: results.filter(row => row.status === 'danger').length,
  }, results }
}
