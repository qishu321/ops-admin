import { ElMessage } from 'element-plus'
import { escapeHtml, reducerLabel, formatReportTime } from '../utils/format.js'

export function useInspectionExport({ inspectionReport, inspectionReportResults }) {
  function exportInspectionReportPdf() {
    const report = inspectionReport.value
    if (!report) {
      ElMessage.warning('请先执行巡检，生成报告后再导出')
      return
    }
    const rows = inspectionReportResults.value.map((item, index) => {
      const stateType = { healthy: 'success', warning: 'warning', danger: 'danger' }[item.status] || 'info'
      const stateText = { healthy: '正常', warning: '待核查', danger: '异常' }[item.status] || item.status
      return `
        <tr>
          <td>${index + 1}</td>
          <td>${escapeHtml(item.datasourceName || report.datasourceName || '-')}</td>
          <td>${escapeHtml(item.title)}<br><small>${escapeHtml(item.category || '')}</small></td>
          <td><span class="status ${stateType}">${escapeHtml(stateText)}</span></td>
          <td>${escapeHtml(item.valueText || '--')}</td>
          <td>${escapeHtml(reducerLabel(item.reducer))}</td>
          <td>${item.seriesCount}</td>
          <td><code>${escapeHtml(item.promql)}</code>${item.error ? `<div class="error">${escapeHtml(item.error)}</div>` : ''}</td>
        </tr>
      `
    }).join('')
    const win = window.open('', '_blank')
    if (!win) {
      ElMessage.warning('浏览器阻止了报告窗口，请允许弹窗后重试')
      return
    }
    win.document.write(`
      <!doctype html>
      <html>
        <head>
          <meta charset="utf-8" />
          <title>${escapeHtml(report.dashboardName)} - 巡检报告 #${report.id}</title>
          <style>
            * { box-sizing: border-box; }
            body { margin: 0; padding: 28px; color: #10213f; font-family: Arial, "Microsoft YaHei", sans-serif; background: #fff; }
            h1 { margin: 0 0 8px; font-size: 26px; }
            .meta { display: flex; gap: 20px; margin-bottom: 22px; color: #64748b; font-size: 13px; }
            .summary { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 22px; }
            .card { padding: 14px; border: 1px solid #dce7f7; border-radius: 10px; background: #f8fbff; }
            .card span { display: block; color: #64748b; font-size: 12px; }
            .card strong { display: block; margin-top: 8px; font-size: 22px; }
            table { width: 100%; border-collapse: collapse; font-size: 12px; }
            th, td { padding: 10px; border: 1px solid #dce7f7; text-align: left; vertical-align: top; }
            th { background: #edf4ff; color: #334155; }
            code { display: block; max-width: 420px; white-space: pre-wrap; word-break: break-all; color: #1d4ed8; font-family: Consolas, Monaco, monospace; }
            .status { display: inline-block; padding: 3px 8px; border-radius: 999px; }
            .success { color: #15803d; background: #dcfce7; }
            .warning { color: #a16207; background: #fef9c3; }
            .danger { color: #b91c1c; background: #fee2e2; }
            .info { color: #475569; background: #e2e8f0; }
            .error { margin-top: 6px; color: #b91c1c; }
            @media print { body { padding: 16px; } .no-print { display: none; } }
          </style>
        </head>
        <body>
          <button class="no-print" onclick="window.print()" style="float:right;padding:8px 14px;">打印 / 保存 PDF</button>
          <h1>${escapeHtml(report.dashboardName)} 巡检报告 #${report.id}</h1>
          <div class="meta">
            <span>巡检范围：${escapeHtml(formatReportTime(report.startAt))} 至 ${escapeHtml(formatReportTime(report.endAt))}</span>
            <span>完成时间：${escapeHtml(formatReportTime(report.completedAt))}</span>
            <span>数据源：${escapeHtml(report.datasourceNames?.length ? report.datasourceNames.join('、') : report.datasourceName || '-')}</span>
            <span>耗时：${report.durationMs || 0} ms</span>
          </div>
          <div class="summary">
            <div class="card"><span>检查结果</span><strong>${report.totalCount}</strong></div>
            <div class="card"><span>正常</span><strong>${report.healthyCount}</strong></div>
            <div class="card"><span>待核查</span><strong>${report.warningCount}</strong></div>
            <div class="card"><span>异常</span><strong>${report.dangerCount}</strong></div>
          </div>
          <table>
            <thead>
              <tr>
                <th>#</th><th>数据源</th><th>巡检项</th><th>状态</th><th>统计值</th><th>统计方式</th><th>序列</th><th>PromQL / 错误</th>
              </tr>
            </thead>
            <tbody>${rows || '<tr><td colspan="8">暂无巡检结果</td></tr>'}</tbody>
          </table>
        </body>
      </html>
    `)
    win.document.close()
    win.focus()
    setTimeout(() => win.print(), 300)
  }

  return {
    exportInspectionReportPdf,
  }
}
