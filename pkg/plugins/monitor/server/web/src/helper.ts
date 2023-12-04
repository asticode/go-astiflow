import type { Flow, Group, Node_ } from "./types/store"

interface formatNumbersResult {
    unit: string
    values: number[]
}

export function formatStat(value: any, unit: string): string | undefined {
  let f = parseFloat(value)
  if (isNaN(f)) return value ? value + unit : undefined

  switch (unit) {
    case 'Bps':
      unit = 'bps'
      f *= 8
      break
  }

  switch (unit) {
    case 'bps':
      return formatNumber(f, [unit, 'k'+unit, 'M'+unit, 'G'+unit])
    case 'ns':
      return formatNumber(f, ['ns', 'µs', 'ms', 's'])
    default:
      return f.toFixed(2) + unit
  }
}

function formatNumber(value: number, units: string[]): string {
  const res = formatNumbers(units, [value])
  return res.values[0].toFixed(2) + res.unit
}

export function formatNumbers(units: string[], values: number[], binary?: boolean): formatNumbersResult {
    const u = binary ? 1024 : 1000
    const exponents: number[] = []
    values.forEach(v => exponents.push(Math.min(Math.floor(Math.log(v) / Math.log(u)), units.length - 1)))
    const exponent = exponents.reduce((prev: number, cur: number): number => Math.min(prev, cur))
    const res: formatNumbersResult = {
        unit: units[exponent],
        values: [],
    }
    values.forEach(v => res.values.push(v / Math.pow(u, exponent)))
    return res
}

export function flowName(f: Flow): string {
    return f.name ?? 'flow_' + f.id
}

export function groupName(g: Group): string {
    return g.metadata.name ?? 'group_' + g.id
}

export function nodeName(n: Node_): string {
    return n.metadata.name ?? 'node_' + n.id
}