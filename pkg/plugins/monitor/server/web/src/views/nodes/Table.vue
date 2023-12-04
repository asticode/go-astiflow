<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { storeService } from '@/services/services'
  import TableComponent, { Order, type Row, type Table } from '@/components/Table.vue'
  import { computed } from 'vue';
  import { groupName, nodeName, formatStat } from '@/helper';

  const s = storeService.get()

  const columnIdGroup = 'group'
  const columnIdNode = 'node'

  const t = computed<Table>((): Table => {
    // Create table
    const t: Table = {
      columns: [
        {
          id: columnIdGroup,
          label: 'Group',
          mergeable: true,
          order: Order.asc,
          sticky: true,
        },
        {
          id: columnIdNode,
          label: 'Node',
          order: Order.asc,
          sticky: true,
        },
      ],
      rows: [],
    }

    // Create stats
    type Stat = {
      count: number,
      description: string,
      id: string,
      label: string,
      name: string,
    }
    let ss: Stat[] = []

    // Loop through groups
    s.flow.groups.forEach(g => {
      // Loop through nodes
      g.nodes.forEach(n => {
        // Filter
        if (!s.nodes.filters.hasNode(g, n)) return

        // Create row
        const r: Row = { cells: {} }
        r.cells[columnIdGroup] = { 
          label: groupName(g),
          value: groupName(g),
        }
        r.cells[columnIdNode] = {
          label: nodeName(n),
          value: nodeName(n),
        }

        // Loop through stats
        n.stats.forEach(stat => {
          // Filter
          if (!s.nodes.filters.hasStat(stat)) return

          // Create id
          const id = 'stat-' + stat.metadata.name

          // Create stat
          let v = ss.find(v => v.name === stat.metadata.name)
          if (!v) {
            v = {
              count: 0,
              description: stat.metadata.description,
              id,
              label: stat.metadata.label,
              name: stat.metadata.name,
            }
            ss.push(v)
          }

          // Update cell
          r.cells[id] = { 
            label: formatStat(stat.value, stat.metadata.unit),
            value: stat.value,
          }

          // Increment
          v.count++
        })

        // Append row
        t.rows.push(r)
      })
    })

    // Order stats
    ss = ss.sort((a: Stat, b: Stat) => b.count - a.count)

    // Add columns
    ss.forEach(s => {
      t.columns.push({
          description: s.description,
          id: s.id,
          label: s.label,
        })
    })
    return t
  })

</script>

<template>
  <TableComponent v-if="s.flow.groups.length > 0" :table="t" />
</template>