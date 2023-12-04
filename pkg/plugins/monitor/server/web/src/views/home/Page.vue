<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
    import { storeService } from '@/services/services';
    import type { HostUsageStatValue } from '@/types/stats';
    import { computed } from 'vue';
    import TableComponent, { Order, type Table } from '@/components/Table.vue'
import { flowName, formatNumbers } from '@/helper';

    const s = storeService.get()

    function hostUsageStatValue(): HostUsageStatValue | undefined {
        const stat = s.flow.stats.find(v => v.metadata.name === "astiflow.host.usage")
        if (!stat || !stat.value) return
        return stat.value as HostUsageStatValue
    }

    const hostUsageTable = computed<Table | undefined>((): Table | undefined => {
        // Get stat value
        const v = hostUsageStatValue()
        if (!v) return

        // Get memory stats
        const units = [ 'B', 'kB', 'MB', 'GB', 'TB' ]
        const processMemoryResult = formatNumbers(units, [v.memory.resident,v.memory.virtual], true)
        const totalMemoryResult = formatNumbers(units, [v.memory.used,v.memory.total], true)

        // Create table
        return {
            columns: [
                {
                    id: "type",
                    label: "Type",
                    order: Order.asc,
                },
                {
                    id: "cpu",
                    label: "CPU",
                },
                {
                    id: "memory",
                    label: "Memory",
                },
            ],
            rows: [
                {
                    cells: {
                        "type": {
                            label: "Process",
                            value: "Process",
                        },
                        "cpu": {
                            label: v.cpu.process !== undefined ? v.cpu.process.toFixed(2) + '%' : undefined,
                            value: v.cpu.process,
                        },
                        "memory": {
                            label: processMemoryResult.values[0].toFixed(2) + '/' + processMemoryResult.values[1].toFixed(2) + processMemoryResult.unit,
                            value: v.memory.resident / v.memory.virtual
                        },
                    }
                },
                {
                    cells: {
                        "type": {
                            label: "Total",
                            value: "Total",
                        },
                        "cpu": {
                            label: v.cpu.total.toFixed(2) + '%',
                            value: v.cpu.total,
                        },
                        "memory": {
                            label: totalMemoryResult.values[0].toFixed(2) + '/' + totalMemoryResult.values[1].toFixed(2) + totalMemoryResult.unit,
                            value: v.memory.used / v.memory.total
                        },
                    }
                },
            ],
        }
    })

    const cpuIndividualTable = computed<Table | undefined>((): Table | undefined => {
        // Get stat value
        const v = hostUsageStatValue()
        if (!v) return

        // Create table
        const t: Table = {
            columns: [
                {
                    id: "cpu",
                    label: "CPU",
                    order: Order.asc,
                },
                {
                    id: "value",
                    label: "Value",
                },
            ],
            rows: [],
        }

        // Loop through cpus
        v.cpu.individual.forEach((v: number, idx: number) => {
            t.rows.push({
                cells: {
                    'cpu': {
                        label: (idx+1).toString(),
                        value: idx,
                    },
                    'value': {
                        label: v.toFixed(2) + '%',
                        value: v,
                    },
                },
            })
        })
        return t
    })

</script>

<template>
    <div id="home">
        <div class="section">
            <div class="title">Flow</div>
            <div class="content">
                <div>{{ flowName(s.flow) }}</div>
                <div v-if="s.flow.description">{{ s.flow.description }}</div>
            </div>
        </div>
        <div class="section" id="host-usage" v-if="hostUsageTable || cpuIndividualTable">
            <div class="title">Host usage</div>
            <div class="content">
                <TableComponent v-if="hostUsageTable" :table="hostUsageTable"/>
                <TableComponent v-if="cpuIndividualTable" :table="cpuIndividualTable"/>
            </div>
        </div>
    </div>
</template>

<style lang="scss" scoped>

    #home {
        padding: 20px;

        .section {
        
            &:not(:last-child) {
                margin-bottom: 20px;
            }

            > .title {
                font-size: 18px;
                font-weight: bold;
                margin-bottom: 10px;
                text-decoration: underline;
            }

            &#host-usage {

                .content > div:not(:last-child) {
                    margin-bottom: 10px;
                }
            }
        }
    }

</style>