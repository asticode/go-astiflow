<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { storeService } from '@/services/services'
  import { computed } from 'vue';
  import Diagram, { type Data, type NodeDataStat } from '@/components/Diagram.vue';
  import { formatStat, groupName, nodeName } from '@/helper';

  const s = storeService.get()

  type fillColor = {
    color: string,
    currentGroupId?: number
    lastGroupId?: number
  }
  const fillColors: fillColor[] = [
    {color: "#f44336"},
    {color: "#e81e63"},
    {color: "#9c27b0"},
    {color: "#673ab7"},
    {color: "#3f51b5"},
    {color: "#2196f3"},
    {color: "#03a9f4"},
    {color: "#00bcd4"},
    {color: "#009688"},
    {color: "#4caf50"},
    {color: "#8bc34a"},
    {color: "#cddc39"},
    {color: "#ead835"},
    {color: "#ffc107"},
    {color: "#ff9800"},
    {color: "#ff5722"},
    {color: "#795548"},
    {color: "#9e9e9e"},
    {color: "#607d8b"},
  ]
  const defaultFillColor: fillColor = { color: "#000000" }

  const d = computed<Data>((): Data => {
    // Create data
    const d: Data = {
      links: [],
      nodes: [],
    }

    // Loop through groups
    const gs: Record<number, boolean> = {}
    const ns: Record<number, boolean> = {}
    s.flow.groups.forEach(g => {
      // Loop through nodes
      g.nodes.forEach(n => {
        // Filter
        if (!s.nodes.filters.hasNode(g, n)) return

        // Add group
        gs[g.id] = true

        // Get node stats
        let ss: NodeDataStat[] = []
        n.stats.forEach(stat => {
          // Filter
          if (!s.nodes.filters.hasStat(stat, false)) return

          // Append
          ss.push({
            label: stat.metadata.label,
            value: formatStat(stat.value, stat.metadata.unit) ?? '',
          })
        })
        ss = ss.sort((a, b): number => {
          if (a.label < b.label) return -1
          else if (a.label > b.label) return 1
          return 0
        })

        // Get fill color
        let fillColor = fillColors
          .filter(c => !c.currentGroupId || c.currentGroupId === g.id)
          .sort((a: fillColor, b: fillColor) => {
            if (a.currentGroupId === g.id) return -1
            if (b.currentGroupId === g.id) return 1
            if (a.lastGroupId === g.id) return -1
            if (b.lastGroupId === g.id) return 1
            return 0
          })[0]
        if (!fillColor) fillColor = defaultFillColor
        else {
          fillColor.currentGroupId = g.id
          fillColor.lastGroupId = g.id
        }

        // Add node
        d.nodes.push({
          fillColor: fillColor.color,
          groupName: groupName(g),
          id: n.id,
          name: nodeName(n),
          stats: ss,
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

    // Reset unused fill colors
    fillColors.forEach(c => { if (c.currentGroupId && !gs[c.currentGroupId]) c.currentGroupId = undefined })
    return d
  })

</script>

<template>
  <Diagram :data="d"/>
</template>