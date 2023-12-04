<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
    import { computed, reactive } from 'vue';

    interface Props {
        table: Table
    }

    const props = defineProps<Props>()
    
    type InternalTable = {
        columns: InternalColumn[],
        rows: InternalRow[],
        sticky?: boolean,
    }

    type InternalColumn = {
        description?: string,
        id: string,
        label: string,
        order?: Order,
    }

    type InternalRow = {
        cells: InternalCell[]
    }

    type InternalCell = {
        label?: string
        rowspan?: number
    }

    type OrderedColumn = {
        id: string,
        order: Order,
    }
    let orderedColumns = reactive<OrderedColumn[]>([])
    props.table.columns.forEach(c => {
        if (c.order) orderedColumns.push({
            id: c.id,
            order: c.order,
        })
    })

    const tables = computed<InternalTable[]>((): InternalTable[] => {
        // Create tables
        const tables: InternalTable[] = [
            {
                columns: [],
                rows: [],
                sticky: true,
            },
            {
                columns: [],
                rows: [],
            },
        ]

        // Order
        // We need to clone rows since sorting is done in place
        let rows: Row[] = [...props.table.rows]
        if (orderedColumns.length > 0) {
            // Sort in place
            rows.sort((a: Row, b: Row): number => {
                // Loop through ordered columns
                for (let i = 0; i < orderedColumns.length; i++) {
                    // Get ordered column
                    const oc = orderedColumns[i]

                    // Get values
                    let av: any = a.cells[oc.id ?? '']?.value
                    let bv: any = b.cells[oc.id ?? '']?.value

                    // Reverse values in case of desc order
                    if (oc.order === 'desc') {
                        const tv: any = av
                        av = bv
                        bv = tv
                    }

                    // Undefined
                    if (typeof av === 'undefined' && typeof bv !== 'undefined') return -1
                    else if (typeof av !== 'undefined' && typeof bv === 'undefined') return 1
                    else if (typeof av === 'undefined' && typeof bv === 'undefined') continue

                    // Number
                    if (typeof av === 'number' && typeof bv === 'number') {
                        const res = av - bv
                        if (res !== 0) return res
                        continue
                    }

                    // Others
                    if (av < bv) return -1
                    else if (av > bv) return 1
                    continue
                }
                return 0
            })
        }

        // Loop through columns
        props.table.columns.forEach((c: Column, columnIdx: number) => {
            // Get table index
            const tableIdx = c.sticky ? 0 : 1

            // Add column
            tables[tableIdx].columns.push({
                description: c.description,
                id: c.id,
                label: c.label,
                order: orderedColumns.find(v => v.id === c.id)?.order,
            })

            // Loop through rows
            let mergePreviousLabel: string | undefined = undefined
            let mergePreviousLabelFirstRowIdx: number | undefined = undefined
            rows.forEach((r: Row, rowIndex: number) => {
                // Make sure row exists
                if (!tables[tableIdx].rows[rowIndex]) tables[tableIdx].rows.push({ cells: [] })

                // Merge
                let label: string | undefined = r.cells[c.id]?.label
                let addCell = true
                if (c.mergeable) {
                    if (mergePreviousLabel === label) {
                        if (mergePreviousLabelFirstRowIdx === undefined) mergePreviousLabelFirstRowIdx = rowIndex-1
                        addCell = false
                        tables[tableIdx].rows[mergePreviousLabelFirstRowIdx].cells[columnIdx].rowspan = (tables[tableIdx].rows[mergePreviousLabelFirstRowIdx].cells[columnIdx].rowspan ?? 1) + 1 
                    } else if (mergePreviousLabelFirstRowIdx !== undefined) mergePreviousLabelFirstRowIdx = undefined
                    mergePreviousLabel = label
                }

                // Add cell
                if (addCell) tables[tableIdx].rows[rowIndex].cells.push({ label })
            })
        })

        // Clean up
        return tables.filter(t => t.columns.length > 0)
    })

    function onOrderColumn(c: InternalColumn) {
        const orderedColumnIdx = orderedColumns.findIndex(v => v.id === c.id)
        if (orderedColumnIdx !== -1) {
            switch (orderedColumns[orderedColumnIdx].order) {
                case Order.asc:
                    orderedColumns[orderedColumnIdx].order = Order.desc
                    break
                default:
                    orderedColumns.splice(orderedColumnIdx, 1)
                    break
            }
        } else {
            orderedColumns.push({
                id: c.id,
                order: Order.asc,
            })
        }
    }

</script>

<script lang="ts">

    export type Table = {
        columns: Column[],
        rows: Row[]
    }

    export enum Order {
        asc = 'asc',
        desc = 'desc'
    }

    type Column = {
        description?: string,
        id: string,
        label: string,
        mergeable?: boolean,
        order?: Order,
        sticky?: boolean,
    }

    export type Row = {
        cells: Record<string, Cell>
    }

    type Cell = {
        label?: string
        value?: any
    }


</script>

<template>
    <div id="table">
        <table :class="{ sticky: table.sticky }" v-for="table, index in tables" v-bind:key="index">
            <thead>
                <tr>
                    <th @click="onOrderColumn(column)" v-for="column, index in table.columns" v-bind:key="index" :title="column.description">
                        <div>
                            <div class="label">
                                {{ column.label }}
                            </div>
                            <div v-if="column.order !== undefined" class="order">
                                <font-awesome-icon :icon="column.order === Order.asc ? 'fa-angle-down' : 'fa-angle-up'"/>
                            </div>
                        </div>
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr v-for="row, index in table.rows" v-bind:key="index">
                    <td v-for="cell, index in row.cells" v-bind:key="index" :rowspan="cell.rowspan">
                        {{ cell.label ?? '' }}
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>

<style lang="scss" scoped>

    #table {
        display: flex;

        table {
            border-collapse: collapse;
            position: relative;
            white-space: nowrap;

            th, td {
                height: 50px;
                padding: 0 10px;
            }

            &.sticky {
                left: 0;
                position: sticky;
                z-index: 2;

                thead {
                    border-right: solid 1px var(--color4);
                }

                tbody {

                    td {
                        text-align: center;
                    }
                }
            }

            &:nth-child(1) thead tr:first-child th:first-child {
                border-radius: 5px 0 0 0;
            }

            &:not(.sticky) thead tr:first-child th:last-child {
                border-radius: 0 5px 0 0;
            }

            thead {
                position: sticky;
                top: 0;
                z-index: 1;

                th {
                    background-color: var(--color1);
                    color: var(--color4);
                    cursor: pointer;

                    > div {
                        align-items: center;
                        display: flex;
                        gap: 5px;
                        justify-content: center;
                    }
                }
            }

            tbody {

                td {
                    border: solid 1px var(--color3);
                    text-align: right;

                    &:empty {
                        background-color: var(--color3);
                        color: var(--color3);
                    }

                    &:not(:empty) {
                        background-color: var(--color4);
                    }
                }
            }
        }
    }

</style>