<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { storeService } from '@/services/services'
  import TextInput from '@/components/TextInput.vue'
  import MultiSelect, { type OptionItem, sortItemOptions } from '@/components/MultiSelect.vue'
  import { computed } from 'vue';
  import { NodesFiltersMode } from '@/types/store';
  import Switch, { type Item } from '@/components/Switch.vue';
import { groupName, nodeName } from '@/helper';

  const s = storeService.get()

  function toggleFiltersVisibility() {
    s.nodes.filters.visible = !s.nodes.filters.visible
  }

  function onFiltersSearchUpdate(value: string) {
    s.nodes.filters.search = value
  }

  const filtersGroupsOptions = computed<OptionItem<string>[]>((): OptionItem<string>[] => {
    const options: OptionItem<string>[] = []
    s.flow.groups.forEach(g => {
      const name = groupName(g)
      if (!options.find(v => v.id === name)) options.push({
        id: name,
        label: name,
        selected: s.nodes.filters.groups.includes(name),
      })
    })
    sortItemOptions(options)
    return options
  })
  function onFiltersGroupsUpdated(values: OptionItem<string>[]) {
    const groups: string[] = []
    values.forEach(o => groups.push(o.id))
    s.nodes.filters.groups = groups
  }

  const filtersNodesOptions = computed<OptionItem<string>[]>((): OptionItem<string>[] => {
    const options: OptionItem<string>[] = []
    s.flow.groups.forEach(g => {
      g.nodes.forEach(n => {
        const name = nodeName(n)
        if (!options.find(v => v.id === name)) options.push({
          id: name,
          label: name,
          selected: s.nodes.filters.nodes.includes(name),
        })
      })
    })
    sortItemOptions(options)
    return options
  })
  function onFiltersNodesUpdated(values: OptionItem<string>[]) {
    const nodes: string[] = []
    values.forEach(n => nodes.push(n.id))
    s.nodes.filters.nodes = nodes
  }

  const filtersStatsOptions = computed<OptionItem<string>[]>((): OptionItem<string>[] => {
    const options: OptionItem<string>[] = []
    s.flow.groups.forEach(g => {
      g.nodes.forEach(n => {
        n.stats.forEach(stat => {
          if (options.find((i: OptionItem<string>) => i.id === stat.metadata.name)) return
          options.push({
            id: stat.metadata.name,
            label: stat.metadata.label,
            selected: s.nodes.filters.stats.includes(stat.metadata.name),
          })
        })
      })
    })
    sortItemOptions(options)
    return options
  })
  function onFiltersStatsUpdated(values: OptionItem<string>[]) {
    const stats: string[] = []
    values.forEach(s => stats.push(s.id))
    s.nodes.filters.stats = stats
  }

  const filtersTagsOptions = computed<OptionItem<string>[]>((): OptionItem<string>[] => {
    const options: OptionItem<string>[] = []
    s.flow.groups.forEach(g => {
      g.metadata.tags?.forEach(tag => {
        if (!options.find((i: OptionItem<string>) => i.id === tag)) options.push({
          id: tag,
          label: tag,
          selected: s.nodes.filters.tags.includes(tag),
        }) 
      })
      g.nodes.forEach(n => {
        n.metadata.tags?.forEach(tag => {
        if (!options.find((i: OptionItem<string>) => i.id === tag)) options.push({
          id: tag,
          label: tag,
          selected: s.nodes.filters.tags.includes(tag),
        }) 
      })
      })
    })
    sortItemOptions(options)
    return options
  })
  function onFiltersTagsUpdated(values: OptionItem<string>[]) {
    const tags: string[] = []
    values.forEach(s => tags.push(s.id))
    s.nodes.filters.tags = tags
  }

  const filtersModeItems: Item<NodesFiltersMode>[] = [
    { label: 'AND', value: NodesFiltersMode.and },
    { label: 'OR', value: NodesFiltersMode.or },
  ]
  function onFiltersModeUpdate(v: NodesFiltersMode){
    s.nodes.filters.mode = v
  }

</script>

<template>
  <div id="filters" :class="{ visible: s.nodes.filters.visible }">
    <div id="visible-toggle">
      <font-awesome-icon @click="toggleFiltersVisibility" :icon="s.nodes.filters.visible ? 'fa-angles-left' : 'fa-angles-right'"/>
    </div>
    <div id="content">
      <div>
        <Switch @update:value="onFiltersModeUpdate" :items="filtersModeItems"/>
      </div>
      <div>
        <TextInput @update:value="onFiltersSearchUpdate" placeholder="Search"/>
      </div>
      <div>
        <MultiSelect @update:values="onFiltersGroupsUpdated" :options="filtersGroupsOptions" placeholder="Groups"/>
      </div>
      <div>
        <MultiSelect @update:values="onFiltersNodesUpdated" :options="filtersNodesOptions" placeholder="Nodes"/>
      </div>
      <div>
        <MultiSelect @update:values="onFiltersStatsUpdated" :options="filtersStatsOptions" placeholder="Stats"/>
      </div>
      <div>
        <MultiSelect @update:values="onFiltersTagsUpdated" :options="filtersTagsOptions" placeholder="Tags"/>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>

  #filters {
    border-right: solid 1px  var(--color3);
    height: 100%;
    min-width: 300px;
    overflow: hidden;
    padding: 10px;
    width: 300px;

    &:not(.visible) {
      min-width: 36px;
      width: 36px;

      > #content {
        width: 0px;
      }
    }

    > #visible-toggle {
      display: flex;
      justify-content: end;
      margin-bottom: 10px;

      > svg {
        cursor: pointer;
      }
    }

    > #content {
      height: 100%;
      overflow: hidden;
      width: 280px;

      > div {

        &:not(:last-child) {
          margin-bottom: 10px;
        }
      }
    }
  }

</style>