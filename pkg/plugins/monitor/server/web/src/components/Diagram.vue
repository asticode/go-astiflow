<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts" setup>
    import { onMounted, watch } from 'vue';
    import go from 'gojs'

    interface Props {
        data: Data
    }

    const props = defineProps<Props>()

    interface tableRow {
        columns: {
            attr: string
            text: string
        }[]
    }

    // eslint-disable-next-line vue/no-dupe-keys
    let data: Data = {
        links: [],
        nodes: [],
    }
    function refresh() {
        // No diagram or model
        if (!d || !m) return

        // Start transaction
        d.startTransaction()

        // Remove all existing nodes/links
        // This is not the most elegant way to refresh, but that's the most efficient
        data.nodes.forEach(n => {
            const d = m?.findNodeDataForKey(nodeKey(n.id))
            if (d) m?.removeNodeData(d)
        })
        data.links.forEach(l => {
            const d = m?.findLinkDataForKey(linkKey(l.from, l.to))
            if (d) m?.removeLinkData(d)
        })

        // Create nodes
        props.data.nodes.forEach(n => {
            const stats: tableRow[] = []
            if (n.stats.length > 0) {
                n.stats.forEach(s => stats.push({
                    columns: [
                        { attr: 'label', text: s.label + ':' },
                        { attr: 'value', text: s.value },
                    ]
                }))
            }
            m?.addNodeData({
                ...n,
                stats,
            })
        })

        // Create links
        props.data.links.forEach(l => m?.addLinkData(l))

        // Commit transaction
        d.commitTransaction('refresh')

        // Update data
        data.links = [...props.data.links]
        data.nodes = [...props.data.nodes]
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
            return nodeKey(a.id)
        }

        // Create diagram
        d = new go.Diagram('diagram', {
            allowSelect: false,
            isReadOnly: true,
            layout: go.GraphObject.make(
                go.TreeLayout,
                {
                    angle: 90,
                    arrangement: go.TreeLayout.ArrangementVertical,
                },
            ),
            linkTemplate: go.GraphObject.make(
                go.Link,
                go.GraphObject.make(go.Shape, { stroke: "#000000" }),
                go.GraphObject.make(go.Shape, {
                    stroke: "#000000",
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
                        strokeWidth: 0,
                    },
                    new go.Binding("fill", "fillColor"),
                ),
                go.GraphObject.make(
                    go.Panel,
                    "Vertical",
                    go.GraphObject.make(
                        go.TextBlock,
                        {
                            font: 'normal 12px Helvetica',
                            margin: new go.Margin(8, 8, 4, 8),
                            stroke: "#ffffff",
                            textAlign: 'center',
                        },
                        new go.Binding("text", "name"),
                    ),
                    go.GraphObject.make(
                        go.TextBlock,
                        {
                            font: 'normal 12px Helvetica',
                            margin: new go.Margin(0, 8, 8, 8),
                            stroke: "#ffffff",
                            textAlign: 'center',
                        },
                        new go.Binding("text", "groupName"),
                    ),
                    go.GraphObject.make(
                        go.Panel,
                        "Table",
                        {
                            margin: new go.Margin(0, 8, 8, 8),
                        },
                        new go.Binding("itemArray", "stats"),
                        new go.Binding("visible", "stats", val => val.length > 0),
                        {
                            name: "TABLE",
                            itemTemplate: go.GraphObject.make(
                                go.Panel,
                                "TableRow",
                                {
                                    margin: new go.Margin(0, 0, 4, 0),
                                },
                                new go.Binding("itemArray", "columns"),
                                {
                                    itemTemplate: go.GraphObject.make(
                                        go.Panel,
                                        new go.Binding("alignment", "attr", a => a === 'label' ? go.Spot.Right : go.Spot.Left),
                                        new go.Binding("column", "attr", a => a === 'label' ? 0 : 1),
                                        go.GraphObject.make(
                                            go.TextBlock,
                                            {
                                                font: 'normal 12px Helvetica',
                                                margin: 2,
                                                stroke: "#ffffff",
                                            },
                                            new go.Binding("isUnderline", "attr", a => a === 'label' ? true : false),
                                            new go.Binding("text").makeTwoWay()),
                                        )
                                }
                            )
                        }
                    ),
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
        d.animationManager.isEnabled = false

        // Refresh
        refresh()
    })

</script>

<script lang="ts">

    export type Data = {
        links: LinkData[]
        nodes: NodeData[]
    }

    type LinkData = {
        from: number,
        to: number,
    }

    type NodeData = {
        fillColor: string,
        groupName: string,
        id: number,
        name: string,
        stats: NodeDataStat[],
    }

    export type NodeDataStat = {
        label: string,
        value: string, 
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