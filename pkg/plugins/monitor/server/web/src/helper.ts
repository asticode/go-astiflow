import type { Flow, Group, Node_ } from "./types/store"

interface formatNumbersResult {
    unit: string
    values: number[]
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