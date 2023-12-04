<script lang="ts" setup>
    import { ref } from 'vue';
    import MultiSelect from 'vue-multiselect'

    interface Props<Id> {
        options: OptionItem<Id>[]
        placeholder: string
    }

    const props = defineProps<Props<any>>()
    const emit = defineEmits(['update:values'])

    let selected: any[] = props.options.map(v => v.selected ? v : undefined).filter(v => v !== undefined)
    let values = ref(selected)
    function onUpdated() {
        emit('update:values', values.value)
    }

</script>

<script lang="ts">

    export type OptionItem<Id> = {
        id: Id
        label: string
        selected?: boolean
    }

    export function sortItemOptions(items: OptionItem<any>[]) {
        items.sort((a: OptionItem<any>, b: OptionItem<any>): number => {
            if (a.label < b.label) return -1
            else if (a.label > b.label) return 1
            return 0
        })
    }

</script>

<template>
    <MultiSelect
        v-model="values"
        @update:model-value="onUpdated"
        :options="options"
        :multiple="true"
        label="label"
        track-by="id"
        :placeholder="placeholder"
        :close-on-select="false"
        :searchable="false"
        selectLabel=""
        selectedLabel=""
        deselectLabel=""
    />
</template>