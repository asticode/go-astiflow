<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
    import { onMounted, watch } from 'vue';
    import go from 'gojs'

    interface Props {
        data: Data
    }

    const props = defineProps<Props>()

    // eslint-disable-next-line vue/no-dupe-keys
    let data: Data = {
        groups: [],
        links: [],
        nodes: [],
    }
    function refresh() {
        // No diagram or model
        if (!d || !m) return

        // Start transaction
        d.startTransaction()

        // Process created groups
        props.data.groups.forEach(g => {
            if (data.groups.find(v => v.id === g.id)) return
            m?.addNodeData({
                ...g,
                isGroup: true,
            })
        })

        // Process created nodes
        props.data.nodes.forEach(n => {
            if (data.nodes.find(v => v.id === n.id)) return
            const g = data.groups.find(v => v.id === n.group)
            m?.addNodeData(n)
        })

        // Process created links
        props.data.links.forEach(l => {
            if (data.links.find(v => v.from === l.from && v.to === l.to)) return
            m?.addLinkData(l)
        })

        // Process removed links
        data.links.forEach(l => {
            if (props.data.links.find(v => v.from === l.from && v.to === l.to)) return
            const d = m?.findLinkDataForKey(linkKey(l.from, l.to))
            if (!d) return
            m?.removeLinkData(d)
        })

        // Process removed nodes
        data.nodes.forEach(n => {
            if (props.data.nodes.find(v => v.id === n.id)) return
            const d = m?.findNodeDataForKey(nodeKey(n.id))
            if (!d) return
            m?.removeNodeData(d)
        })

        // Process removed groups
        data.groups.forEach(g => {
            if (props.data.groups.find(v => v.id === g.id)) return
            const d = m?.findNodeDataForKey(groupKey(g.id))
            if (!d) return
            m?.removeNodeData(d)
        })

        // Commit transaction
        d.commitTransaction('refresh')

        // Update data
        data.groups = [...props.data.groups]
        data.links = [...props.data.links]
        data.nodes = [...props.data.nodes]
    }

    function groupKey(id: number): string {
        return 'group-' + id
    }

    function nodeKey(id: number): string {
        return 'node-' + id
    }

    function linkKey(from: number, to: number): string {
        return from + '->' + to
    }

    watch(() => props.data, () => refresh())

    let d: go.Diagram | undefined = undefined
    let m: go.GraphLinksModel | undefined = undefined
    onMounted(() => {
        // Create model
        m = new go.GraphLinksModel()
        m.linkKeyProperty = (a: go.ObjectData): string => {
            return linkKey(a.from, a.to)
        }
        m.linkFromKeyProperty = (a: go.ObjectData): string => {
            return nodeKey(a.from)
        }
        m.linkToKeyProperty = (a: go.ObjectData): string => {
            return nodeKey(a.to)
        }
        m.nodeKeyProperty = (a: go.ObjectData): string => {
            if (a.isGroup) return groupKey(a.id)
            return nodeKey(a.id)
        }
        m.nodeGroupKeyProperty = (a: go.ObjectData): string => {
            return groupKey(a.group)
        }

        // Create diagram
        d = new go.Diagram('diagram', {
            allowSelect: false,
            groupTemplate: go.GraphObject.make(
                go.Group,
                "Vertical",
                go.GraphObject.make(
                    go.TextBlock,
                    {
                        font: "bold 14px Helvetica",
                        margin: 2,
                        stroke: "#353740",
                    },
                    new go.Binding("text"),
                ),
                go.GraphObject.make(
                    go.Panel,
                    "Auto",
                    go.GraphObject.make(
                        go.Shape,
                        "RoundedRectangle",
                        {
                            fill: "#4EBD8C33",
                            strokeWidth: 0,
                        },
                    ),
                    go.GraphObject.make(
                        go.Placeholder,
                        { padding: 5},
                    )
                ),
            ),
            isReadOnly: true,
            layout: go.GraphObject.make(
                go.TreeLayout,
                {
                    angle: 90,
                    arrangement: go.TreeLayout.ArrangementHorizontal,
                },
            ),
            linkTemplate: go.GraphObject.make(
                go.Link,
                go.GraphObject.make(go.Shape, { stroke: "#4EBD8C" }),
                go.GraphObject.make(go.Shape, {
                    stroke: "#4EBD8C",
                    toArrow: "OpenTriangle",
                })
            ),
            model: m,
            nodeTemplate: go.GraphObject.make(
                go.Node, 
                "Auto",
                go.GraphObject.make(
                    go.Shape,
                    "RoundedRectangle",
                    {
                        fill: "#4EBD8C",
                        strokeWidth: 0,
                    },
                    new go.Binding("fill", "color"),
                ),
                go.GraphObject.make(
                    go.TextBlock,
                    {
                        font: 'normal 12px Helvetica',
                        margin: 8,
                        stroke: "#ffffff",
                    },
                    new go.Binding("text").makeTwoWay(),
                ),
                {
                    toolTip: go.GraphObject.make(
                        go.Adornment,
                        "Spot",
                        {
                            background: "transparent",
                        }, 
                        go.GraphObject.make(go.Placeholder, { padding: 10 }),
                        go.GraphObject.make(
                            go.TextBlock,
                            {
                                alignment: go.Spot.Bottom,
                                stroke: "#4EBD8C"
                            },
                            new go.Binding("text", "", n => "id: " + n.id),
                        ),
                    ),
                }
            ),
        })

        // Refresh
        refresh()
    })

</script>

<script lang="ts">

    export type Data = {
        groups: GroupData[]
        links: LinkData[]
        nodes: NodeData[]
    }

    type GroupData = {
        id: number,
        text: string,
    }

    type LinkData = {
        from: number,
        to: number,
    }

    type NodeData = {
        group: number,
        id: number,
        text: string,
    }

</script>

<template>
    <div id="diagram"/>
</template>

<style scoped>

    #diagram {
        height: 100%;
    }

</style>