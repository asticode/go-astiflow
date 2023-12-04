<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { storeService } from '@/services/services'
  import { computed } from 'vue';
  import Diagram, { type Data } from '@/components/Diagram.vue';
  import type { Group, Node_ } from '@/types/store'
import { groupName, nodeName } from '@/helper';

  const s = storeService.get()

  const d = computed<Data>((): Data => {
    // Create data
    const d: Data = {
      groups: [],
      links: [],
      nodes: [],
    }

    // Loop through groups
    const gs: Record<number, boolean> = {}
    const ns: Record<number, boolean> = {}
    const groups = [...s.flow.groups]
    groups.sort(sort).forEach(g => {
      // Loop through nodes
      const nodes = [...g.nodes]
      nodes.sort(sort).forEach(n => {
        // Filter
        if (!s.nodes.filters.hasNode(g, n)) return

        // Add group
        if (!gs[g.id]) {
          d.groups.push({
            id: g.id,
            text: groupName(g),
          })
          gs[g.id] = true
        }

        // Add node
        d.nodes.push({
          group: g.id,
          id: n.id,
          text: nodeName(n),
        })
        ns[n.id] = true
      })
    })

    // Loop through connections
    s.flow.connections.forEach(c => {
      // One node is filtered
      if (!ns[c.from] || !ns[c.to]) return

      // Add link to data
      d.links.push({
        from: c.from,
        to: c.to,
      })
    })
    return d
  })

  function sort(a: Node_ | Group, b: Node_ | Group): number {
    if (a.id < b.id) return -1
    else if (a.id > b.id) return 1
    return 0
  }

</script>

<template>
  <Diagram :data="d"/>
</template>