/** Monotone cubic interpolation: pass every sample without inventing local extrema. */
export function smoothPath(points) {
  const samples = points.trim().split(/\s+/).filter(Boolean).map(pair => pair.split(',').map(Number))
  if (!samples.length || samples.some(p => p.length !== 2 || !p.every(Number.isFinite))) return ''
  if (samples.length === 1) return `M ${samples[0].join(' ')}`
  const slopes = samples.slice(1).map(([x, y], i) => (y - samples[i][1]) / (x - samples[i][0]))
  const tangents = samples.map((_, i) => i === 0 ? slopes[0] : i === samples.length - 1 ? slopes[i - 1] : (slopes[i - 1] + slopes[i]) / 2)
  slopes.forEach((slope, i) => {
    if (!Number.isFinite(slope) || slope === 0) { tangents[i] = 0; tangents[i + 1] = 0; return }
    if (tangents[i] / slope < 0) tangents[i] = 0
    if (tangents[i + 1] / slope < 0) tangents[i + 1] = 0
    const magnitude = Math.hypot(tangents[i] / slope, tangents[i + 1] / slope)
    if (magnitude > 3) {
      tangents[i] *= 3 / magnitude
      tangents[i + 1] *= 3 / magnitude
    }
  })
  return samples.slice(1).reduce((d, [x, y], i) => {
    const [previousX, previousY] = samples[i]
    const step = (x - previousX) / 3
    return `${d} C ${previousX + step} ${previousY + tangents[i] * step}, ${x - step} ${y - tangents[i + 1] * step}, ${x} ${y}`
  }, `M ${samples[0].join(' ')}`)
}

export function areaPath(points) {
  const line = smoothPath(points)
  if (!line) return ''
  const samples = points.trim().split(/\s+/)
  return `${line} L ${samples.at(-1).split(',')[0]} 52 L ${samples[0].split(',')[0]} 52 Z`
}
