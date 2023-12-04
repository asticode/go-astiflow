import { NodesFilters, type Store } from "@/types/store";
import { reactive, type UnwrapNestedRefs } from 'vue'

export class StoreService {

    private s: UnwrapNestedRefs<Store>

    constructor() {
        this.s = reactive({
            flow: {
                connections: [],
                groups: [],
                stats: [],
            },
            nodes: {
                filters: new NodesFilters(),
            },
            replay: {
                ats: [],
                open: {}
            },
        })
    }

    get(): UnwrapNestedRefs<Store> {
        return this.s
    }

}

