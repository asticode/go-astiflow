<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { RouteParam, RouteParamVisualization } from '@/types/router';
  import Diagram from './Diagram.vue'
  import Filters from './Filters.vue'
  import Table from './Table.vue'

</script>

<template>
  <Filters/>
  <div id="content">
    <div>
      <Table v-if="$route.params[RouteParam.visualization] === RouteParamVisualization.table"/>
      <Diagram v-else-if="$route.params[RouteParam.visualization] === RouteParamVisualization.diagram"/>
    </div>
  </div>
</template>

<style lang="scss" scoped>

  #content {
    display: flex;
    flex-grow: 1;
    height: 100%;
    overflow: hidden;

    > div {
      margin: 20px;
      overflow: auto;
      width: 100%;
    }
  }

</style>