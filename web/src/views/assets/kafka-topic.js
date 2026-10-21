export function formatKafkaCount(value) {
 if (value === undefined || value === null || value === '—') return '—'
 try { return BigInt(value).toLocaleString('en-US') } catch { return String(value) }
}
export function kafkaOffsetAtLeast(value, target) { try { return BigInt(value) >= BigInt(target) } catch { return false } }
export function kafkaOffsetPercent(value, maximum) { try { const n = BigInt(maximum); return n > 0n ? Number(BigInt(value) * 1000n / n) / 10 : 0 } catch { return 0 } }
export function kafkaPayload(value, pretty = false) {
 if (!value || value.null) return 'null'
 if (pretty && value.encoding !== 'base64') { try { return JSON.stringify(JSON.parse(value.text), null, 2) } catch {} }
 return value.text ?? ''
}
export const kafkaConfigSource = source => ({0:'未知',1:'Topic 配置',2:'动态 Broker 配置',3:'动态默认 Broker 配置',4:'Broker 配置',5:'默认配置',6:'动态 Broker Logger 配置'})[source] || '未知'
