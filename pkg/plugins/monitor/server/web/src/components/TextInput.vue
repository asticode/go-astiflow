<script lang="ts" setup>
    import { computed } from 'vue';

    interface Props {
        placeholder: string,
        value?: string,
    }

    const props = defineProps<Props>()
    const emit = defineEmits(['update:value'])

    const value = computed({
        get() {
            return props.value
        },
        set(value) {
            emit('update:value', value)
        }
    })

</script>

<template>
    <input type="text" :placeholder="placeholder" v-model="value"/>
</template>

<style lang="scss" scoped>

    input {
        border-radius: 5px;
        border: solid 1px var(--color3);
        font-size: 16px;
        outline: 0;
        padding: 12px 15px;
        width: 100%;
    }

</style>