export type ApiCatchUp = {
    flow: {
        description?: string
        id: number,
        name?: string
    }
} & ApiDelta

export type ApiDelta = {
    at: number,
    connected_nodes?: ApiDeltaConnection[],
    disconnected_nodes?: ApiDeltaConnection[],
    done_groups?: number[],
    done_nodes?: number[],
    new_stats?: ApiDeltaStat[],
    started_groups?: ApiDeltaGroup[],
    started_nodes?: ApiDeltaNode[],
    stat_values?: Record<string, unknown>,
}

type ApiDeltaConnection = {
    from: number,
    to: number,
}

type ApiDeltaGroup = {
    id: number,
    metadata: ApiDeltaMetadata,
}

type ApiDeltaMetadata = {
    description?: string,
    name?: string,
    tags?: string[],
}

type ApiDeltaNode = {
    group_id: number,
    id: number,
    metadata: ApiDeltaMetadata,
}

type ApiDeltaStat = {
    id: number,
    metadata: ApiDeltaStatMetadata,
    node_id?: number,
}

type ApiDeltaStatMetadata = {
    description: string,
    label: string,
    name: string,
    unit: string,
}