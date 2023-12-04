import { groupName, nodeName } from "@/helper"

export type Store = {
    flow: Flow,
    nodes: Nodes,
    replay: Replay,
}

export type Flow = {
    connections: Connection[],
    description?: string,
    groups: Group[],
    id: number,
    name?: string,
    stats: Stat[],
}

type Connection = {
    from: number,
    to: number,
}

export type Group = {
    id: number,
    metadata: Metadata,
    nodes: Node_[],
}

export type Metadata = {
    description?: string,
    name?: string,
    tags?: string[],
}

export type Node_ = {
    id: number,
    metadata: Metadata,
    stats: Stat[]
}

type Nodes = {
    filters: NodesFilters
}

export enum NodesFiltersMode {
    and = 'and',
    or = 'or',
}

export class NodesFilters {

    groups: string[] = []
    mode: NodesFiltersMode = NodesFiltersMode.and
    nodes: string[] = []
    search: string = ""
    stats: string[] = []
    tags: string[] = []
    visible: boolean = false

    hasNode(g: Group, n: Node_): boolean {
      const filters: boolean[] = []
      if (this.groups.length > 0) filters.push(this.groups.includes(groupName(g)))
      if (this.nodes.length > 0) filters.push(this.nodes.includes(nodeName(n)))
      if (this.search !== '') filters.push(!!groupName(g).includes(this.search) || !!nodeName(n).includes(this.search))
      if (this.tags.length > 0) filters.push(this.tags.some(tag => !!g.metadata.tags?.includes(tag) || !!n.metadata.tags?.includes(tag)))
      if (filters.length > 0) {
        if (this.mode === NodesFiltersMode.and && filters.includes(false)) return false
        else if (this.mode === NodesFiltersMode.or && !filters.includes(true)) return false
      }
      return true
    }
  
    hasStat(s: Stat, emptyValue = true): boolean {
      if (this.stats.length === 0) return emptyValue
      const filters: boolean[] = []
      filters.push(this.stats.includes(s.metadata.name))
      if (this.mode === NodesFiltersMode.and && filters.includes(false)) return false
      else if (this.mode === NodesFiltersMode.or && !filters.includes(true)) return false
      return true
    }

}

export type Stat = {
    id: number,
    metadata: StatMetadata
    value?: unknown
}

type StatMetadata = {
    description: string,
    label: string,
    name: string,
    unit: string 
}

type Replay = {
    ats: number[],
    currentIdx?: number,
    open: {
        done?: boolean,
        error?: string,
        progress?: {
            elapsed: number,
            label: string,
        }
    },
}