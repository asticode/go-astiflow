<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
  import { computed, ref } from 'vue';
  import { RouterLink, type RouteParamsRaw, useRoute } from 'vue-router'
  import { replayService, storeService } from '@/services/services'
  import ProgressBar, { type Step, type Steps } from './ProgressBar.vue';
  import { RouteParam, RouteParamVisualization } from '@/types/router';

  const s = storeService.get()

  const fileInput = ref<HTMLInputElement | null>(null)

  function onOpenFileClick() {
    fileInput.value?.click()
  }

  interface FileEvent {
    files: File[]
  }

  function onFileOpened(e: Event) {
    const f = (e.target as unknown as FileEvent)?.files[0]
    if (!f) return
    replayService.open(f)
  }

  function onClearReplay() {
    replayService.clear()
  }

  function formatDate(i: number): string {
    const d = new Date(i*1000)
    return d.getHours().toString().padStart(2, '0')+':'+d.getMinutes().toString().padStart(2, '0')+':'+d.getSeconds().toString().padStart(2, '0')
  }

  const replayCurrentLabel = computed<string | undefined>((): string | undefined => {
    if (s.replay.currentIdx === undefined) return
    return formatDate(s.replay.ats[s.replay.currentIdx])
  })

  const replayCurrentElapsed = computed<number>((): number => {
    if (s.replay.currentIdx === undefined) return 0
    const max = s.replay.ats[s.replay.ats.length - 1]
    const min = s.replay.ats[0]
    const current = s.replay.ats[s.replay.currentIdx]
    return (current - min) / (max - min) * 100
  })

  const replayCurrentItems = computed<Steps<number>>((): Steps<number> => {
    return {
      items: s.replay.ats,
      label: formatDate,
    }
  })

  function onProgressBarClick(step: Step<number>) {
    replayService.seek(step.index)
  }

  interface HeaderItem {
    icon: string
    name: string
    params?: RouteParamsRaw
  }
  const headerItems: HeaderItem[] = [
    {
      icon: 'house',
      name: 'home',
    },
    {
      icon: 'table',
      name: 'nodes',
      params: headerItemParams(RouteParam.visualization, RouteParamVisualization.table),
    },
    {
      icon: 'diagram-project',
      name: 'nodes',
      params: headerItemParams(RouteParam.visualization, RouteParamVisualization.diagram),
    },
  ]

  function headerItemParams(k: RouteParam, v: string): RouteParamsRaw {
    const ps: RouteParamsRaw = {}
    ps[k] = v
    return ps
  }

  const r = useRoute()
  function isHeaderItemSelected(i: HeaderItem): boolean {
    if (r.name !== i.name) return false
    const rks = Object.keys(r.params)
    const ips = i.params ?? {}
    const iks = Object.keys(ips)
    if (rks.length !== iks.length) return false
    for (let idx = 0; idx < rks.length; idx++) {
      if (r.params[rks[idx]] !== ips[rks[idx]]) return false
    }
    return true
  }

</script>

<template>
  <header>
    <div id="icons">
      <div v-for="item, index in headerItems" v-bind:key="index" :class="{ selected: isHeaderItemSelected(item) }">
        <RouterLink :to="{ name: item.name, params: item.params }"><font-awesome-icon :icon="'fa-'+item.icon"/></RouterLink>
      </div>
      <div @click="onOpenFileClick" v-if="!s.replay.open.done">
        <font-awesome-icon icon="fa-file"/>
        <input @change="onFileOpened" type="file" ref="fileInput" accept="text/csv"/>
      </div>
      <div v-else @click="onClearReplay">
        <font-awesome-icon icon="fa-file-circle-xmark"/>
      </div>
    </div>
    <div id="progress">
      <ProgressBar v-if="s.replay.currentIdx !== undefined" @click="onProgressBarClick" :elapsed="replayCurrentElapsed" :total="100" :label="replayCurrentLabel" :steps="replayCurrentItems"/>
      <ProgressBar v-else-if="s.replay.open.progress !== undefined" :elapsed="s.replay.open.progress.elapsed" :error="s.replay.open.error" :total="100" :label="s.replay.open.progress.label"/>
    </div>
  </header>
</template>

<style lang="scss" scoped>

  header {
    border-bottom: solid 1px var(--color3);
    display:flex;

    > #icons {
      align-items: center;
      border-right: solid 1px var(--color3);
      display:flex;
      height: 42px;
      padding: 0 20px;

      > div {
        font-size: 22px;

        &:not(:last-child) {
          margin-right: 15px;
        }

        svg {
          color: var(--color2);
          cursor: pointer;
        }

        &.selected svg, &:hover svg {
          color: var(--color1);
        }

        &.selected svg {
          cursor: default;
        }

        input[type=file] {
          display: none;
        }
      }
    }

    > #progress {
      flex-grow: 1;
      padding: 6px 20px 6px 20px;
    }
  }

</style>