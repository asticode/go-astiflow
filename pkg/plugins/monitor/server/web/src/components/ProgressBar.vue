<script lang="ts" setup>
    import { computed, ref } from 'vue';

    interface Props {
        elapsed: number
        error?: string
        label?: string
        steps?: Steps<any>
        total: number
    }

    const props = defineProps<Props>()
    const emit = defineEmits(['click'])
    
    const width = computed(() => {
        let w = 0
        if (hovered.value && props.steps) {
            const max = props.steps.items[props.steps.items.length - 1]
            const min = props.steps.items[0]
            w = (hovered.value.value - min) / (max - min) * 100
        } else if (props.total > 0) {
            w = props.elapsed / props.total * 100
        }
        return w
    })
    const label = computed<string | undefined>((): string | undefined => {
        if (hovered.value && props.steps) return props.steps.label(hovered.value.value)
        return props.error ?? props.label
    })

    const progressBar = ref<HTMLDivElement | null>(null)
    const hovered = ref<Step<any> | undefined>()

    function onMouseMove(e: MouseEvent) {
        if (!props.steps || !progressBar.value) return
        const max = props.steps.items[props.steps.items.length - 1]
        const min = props.steps.items[0]
        const clientXPercentage = (e.clientX - progressBar.value.offsetLeft) / progressBar.value.clientWidth
        const goal = clientXPercentage * (max - min) + min
        const value = props.steps.items.reduce((prev: any, curr: any) => {
            return (Math.abs(curr - goal) < Math.abs(prev - goal) ? curr : prev);
        })
        hovered.value = {
            index: props.steps.items.findIndex(v => v === value),
            value: value,
        }
    }

    function onMouseLeave() {
        hovered.value = undefined
    }

    function onClick() {
        if (!hovered.value) return
        emit('click', hovered.value)
    }

</script>

<script lang="ts">

    export interface Steps<T> {
        label: (v: any) => string
        items: T[]
    }

    export interface Step<T> {
        index: number,
        value: T,
    }

</script>

<template>
    <div id="progress-bar" :class="{ hoverable: props.steps }" @click="onClick" @mousemove="onMouseMove" @mouseleave="onMouseLeave" ref="progressBar">
        <div id="bar" :class="{ error: props.error }" :style="{ width: width + '%' }"></div>
        <div v-if="label" id="label">{{ label }}</div>
    </div>
</template>

<style lang="scss" scoped>

    #progress-bar {
        background-color: var(--color2);
        border-radius: 20px;
        height: 30px;
        overflow: hidden;
        position: relative;
        width: 100%;

        &.hoverable {
            cursor: pointer;

            > #bar {
                transition: none;
            }
        }

        > #bar {
            background-color: var(--color1);
            border-radius: 20px;
            height: 100%;
            /* //!\\ When opening replay, it doesn't go up to 100% when transition is on */
            transition: none;

            &.error {
                background-color: var(--color5);
            }
        }

        > #label {
            align-items: center;
            display: flex;
            color: var(--color4);
            height: 100%;
            justify-content: center;
            left: 0;
            position: absolute;
            top: 0;
            width: 100%;
        }
    }

</style>