<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>

    interface Props {
        label: string
    }

    defineProps<Props>()

</script>

<template>
    <button>{{ label }}</button>
</template>

<style lang="scss" scoped>

    button {
        background-color: var(--color1);
        border: solid 1px var(--color1);
        border-radius: 4px;
        color: var(--color4);
        padding: 9px 12px;
        width: 100%;

        &:hover {
            cursor: pointer;
        }
    }

</style>