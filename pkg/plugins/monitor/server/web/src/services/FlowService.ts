import type { Stat } from "@/types/store";
import { DisabledError, type ApiService } from "./ApiService";
import type { StoreService } from "./StoreService";
import type { ApiCatchUp, ApiDelta } from "@/types/api";
import type { PushService } from "./PushService";
import { flowName } from "@/helper";

export class FlowService {

    constructor(
        private apiService: ApiService,
        pushService: PushService,
        private storeService: StoreService,
    ) {
        pushService.onDelta(this.onDelta)
    }

    async catchUp(d?: ApiCatchUp) {
        this.clear()
        if (!d) {
            try {
                d = await this.apiService.catchUp()
            } catch (err) {
                if (err instanceof DisabledError) return
                throw err
            }
        }
        this.storeService.get().flow.description = d.flow.description
        this.storeService.get().flow.id = d.flow.id
        this.storeService.get().flow.name = d.flow.name
        document.title = flowName(this.storeService.get().flow)
        this.onDelta(d)
    }

    clear() {
        const f = this.storeService.get().flow
        f.connections = []
        f.groups = []
        f.stats = []
    }

    onDelta = (d: ApiDelta): void => {
        // Get store
        const store = this.storeService.get()

        // Connected nodes
        d.connected_nodes?.forEach(c => {
            store.flow.connections.push(c)
        })

        // Started groups
        d.started_groups?.forEach(g => {
            store.flow.groups.push({
                id: g.id,
                metadata: {
                    description: g.metadata.description,
                    name: g.metadata.name,
                    tags: g.metadata.tags,
                },
                nodes: [],
            })
        })

        // Started nodes
        d.started_nodes?.forEach(n => {
            store.flow.groups.find(g => g.id === n.group_id)?.nodes.push({
                id: n.id,
                metadata: {
                    description: n.metadata.description,
                    name: n.metadata.name,
                    tags: n.metadata.tags,
                },
                stats: [],
            })
        })

        // Disconnected nodes
        d.disconnected_nodes?.forEach(c => {
            store.flow.connections = store.flow.connections.filter(v => v.from !== c.from && v.to !== c.to)
        })

        // Done groups
        d.done_groups?.forEach(id => {
            const idx = store.flow.groups.findIndex(g => g.id === id)
            if (idx === -1) return
            store.flow.groups.splice(idx, 1)
        })

        // Done nodes
        d.done_nodes?.forEach(id => {
            store.flow.groups.forEach(g => {
                const idx = g.nodes.findIndex(n => n.id === id)
                if (idx === -1) return
                g.nodes.splice(idx, 1)
            })
        })

        // New stats
        d.new_stats?.forEach(s => {
            const v: Stat = {
                id: s.id,
                metadata: {
                    description: s.metadata.description,
                    label: s.metadata.label,
                    name: s.metadata.name,
                    unit: s.metadata.unit,
                },
            }

            if (s.node_id) store.flow.groups.forEach(g => {
                g.nodes.find(n => n.id === s.node_id)?.stats.push(v)
            })  
            else store.flow.stats.push(v)
        })

        // Stat values
        const statValues = d.stat_values ?? {}
        Object.keys(statValues).forEach(k => {
            const statId: number = parseInt(k)
            const value = statValues[k]
            store.flow.stats.forEach(s => {
                if (s.id === statId) s.value = value
            })
            store.flow.groups.forEach(g => {
                g.nodes.forEach(n => {
                    n.stats.forEach(s => {
                        if (s.id === statId) s.value = value
                    })
                })
            })
        })
    }

}

