// Formats large counters with Chinese units so metric cards never overflow.
// Values below 10000 stay exact; larger values use 万 / 亿 with two decimals.
// Examples: 12345 -> 1.23万, 1234000 -> 123.40万, 123456789 -> 1.23亿.
export function formatCompactNumber(value?: number | null): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  const sign = value < 0 ? '-' : ''
  const abs = Math.abs(value)
  if (abs < 10_000) return sign + abs.toLocaleString('zh-CN')
  if (abs >= 100_000_000) return sign + (abs / 100_000_000).toFixed(2) + '亿'
  const wan = Math.round((abs / 10_000) * 100) / 100
  // 99,999,999 rounds to 10000.00万; promote it to 亿 instead.
  if (wan >= 10_000) return sign + (abs / 100_000_000).toFixed(2) + '亿'
  return sign + wan.toFixed(2) + '万'
}
