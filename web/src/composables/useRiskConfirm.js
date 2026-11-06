import { ElMessageBox } from 'element-plus'
import { getLocale, tr } from '../utils/i18n'

/**
 * A consistent confirmation gate for production and destructive operations.
 * The backend still owns authorization; this supplies an explicit, auditable
 * client-side acknowledgement before a request is sent.
 */
export async function confirmRiskOperation({
  operation,
  targetSummary,
  targetCount = 0,
  production = false,
  destructive = false
}) {
  const acknowledgement = getLocale() === 'en-US'
    ? production ? 'PRODUCTION' : 'CONFIRM'
    : production ? '生产环境' : destructive ? '我确认' : '确认执行'
  const riskText = [
    production ? tr('目标包含生产环境。') : '',
    destructive ? tr('该操作可能修改或删除现有数据。') : '',
    targetCount ? tr('本次将作用于 {count} 个目标。', { count: targetCount }) : ''
  ].filter(Boolean).join(' ')

  await ElMessageBox.prompt(
    tr('{risk}\n\n操作：{operation}\n目标：{target}\n\n请输入“{ack}”后继续。', { risk: riskText, operation, target: targetSummary, ack: acknowledgement }),
    production ? tr('生产操作确认') : tr('高风险操作确认'),
    {
      type: 'warning',
      inputPlaceholder: acknowledgement,
      inputValidator: (value) => value === acknowledgement || tr('请输入“{ack}”以确认', { ack: acknowledgement }),
      confirmButtonText: tr('确认并继续'),
      cancelButtonText: tr('取消'),
      closeOnClickModal: false,
      closeOnPressEscape: false
    }
  )
}
