<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
    import { ref } from 'vue';

    interface Props {
        items: Item<any>[]
    }

    export interface Item<V> {
        label: string
        value: V
    }

    const props = defineProps<Props>()
    const emit = defineEmits(['update:value'])

    const active = ref(false)

    function activeItem(): Item<any> {
        return active.value ? props.items[1] : props.items[0]
    }

    function onClick() {
        active.value = !active.value
        emit('update:value', activeItem().value)
    }

</script>

<template>
    <div id="switch" @click="onClick" :class="active ? 'on' : 'off'">
        <div class="knob">{{ activeItem().label }}</div>
    </div>
</template>

<style lang="scss" scoped>

    #switch {
        border: solid 1px var(--color3);
        border-radius: 45px;
        cursor: pointer;
        height: 45px;
        position: relative;
        width: 80px;

        > div {
            align-items: center;
            background-color: var(--color1);
            border-radius: 100%;
            color: var(--color4);
            display: flex;
            font-size: 10px;
            height: 35px;
            justify-content: center;
            left: 5px;
            position: absolute;
            top: 5px;
            width: 35px;
        }

        &.on {

            > div {
                left: calc(100% - 5px);
                transform: translateX(-100%);
            }
        }
    }

</style>