<script lang="ts" setup>
  import { RouterView } from 'vue-router'
  import { onMounted } from 'vue'
  import { flowService, loggerService } from '@/services/services'
  import Header from './components/Header.vue';

  onMounted(async () => {
    flowService.catchUp()
      .catch(err => loggerService.error('catching up failed', err))
  })
</script>

<template>
  <Header/>
  <main>
    <RouterView/>
  </main>
</template>

<style lang="scss" scoped>

  main {
    display: flex;
    flex-grow: 1;
    overflow: auto;
  }

</style>
