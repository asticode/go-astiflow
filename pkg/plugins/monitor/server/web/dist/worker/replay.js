class Service {

    constructor() {
        this.resetState()
    }

    resetState() {
        this.state = {
            groups: {},
            nodes: {},
            stats: {},
            values: [],
        }
        this.resetStateCurrent()
    }

    resetStateCurrent() {
        this.state.current = {
            groups: {},
            idx: null,
            nodes: {},
            stats: {},
        }
    }

    clear() {
        // Nothing to do
        if (this.state.current.idx === null) return

        // Reset state
        this.resetState()

        // Send cleared
        this.send('cleared')
    }

    open(f) {
        // Clear
        this.clear()

        // Log
        this.sendLog('opening')

        // Progress
        this.sendProgress('open', 0, 'Opening...')

        // Create file reader
        const r = new FileReader()
        r.addEventListener('load', this.onFileLoad)
        r.addEventListener('progress', this.onFileProgress)
        
        // Read
        r.readAsText(f)
    }

    onFileProgress = (e) => {
        // Send progress
        this.sendProgress('open', (e.loaded / e.total * 100) * 0.5)
    }

    onFileLoad = (e) => {
        // Progress
        this.sendProgress('open', 50, 'Parsing...')

        // Log
        this.sendLog('parsing')

        // Invalid result
        if (typeof e.target?.result !== 'string') {
            this.sendError('invalid result type')
            return
        }

        // Loop through lines
        const lines = e.target.result.split(/\r\n|\n/)
        const ats = []
        let init = {}
        lines.forEach((line, idx) => {
            try {
                // Parse line
                if (line.length > 0) {
                    // Parse
                    const d = JSON.parse(line)

                    // Init
                    if (idx === 0) {
                        init = d
                        return
                    }

                    // Started groups
                    d.started_groups?.forEach(g => this.state.groups[g.id] = g)

                    // Started nodes
                    d.started_nodes?.forEach(n => this.state.nodes[n.id] = n)

                    // New stats
                    d.new_stats?.forEach(s => this.state.stats[s.id] = s)

                    // Create value
                    const v = { 
                        at: d.at,
                        connections: this.state.values.length > 0 ? this.state.values[this.state.values.length - 1].connections : [],
                        stats: {},
                    }

                    // Connected/disconnected nodes
                    if (d.connected_nodes || d.disconnected_nodes) {
                        // Clone connections
                        v.connections = [...v.connections]

                        // Connected nodes
                        d.connected_nodes?.forEach(c => v.connections.push(c))

                        // Disconnected nodes
                        d.disconnected_nodes?.forEach(c => v.connections = v.connections.filter(i => i.from !== c.from || i.to !== c.to))
                    }

                    // Stat values
                    if (d.stat_values && Object.keys(d.stat_values).length > 0) {
                        v.stats = d.stat_values
                    }

                    // Append value
                    ats.push(d.at)
                    this.state.values.push(v)
                }
            } catch (err) {
                // Log
                this.sendLog(`parsing line "${line}" failed: ${err}`)
            } finally {
                // Progress
                this.sendProgress('open', 50 + ((idx+1)/lines.length) * 50)
            }
        })

        // Get next delta
        const d = this.nextDelta()

        // Log
        this.sendLog('parsed')

        // Opened
        this.sendOpened(ats, { ...d, ...init })
    }

    next() {
        // Get next delta
        const d = this.nextDelta()
        if (!d) return

        // Send delta
        this.sendDelta(d)
    }

    nextDelta() {
        // Get idx
        const idx = this.state.current.idx !== null ? this.state.current.idx+1 : 0

        // Build delta
        return this.buildDelta(idx)
    }

    previous() {
        // Get previous delta
        const d = this.previousDelta()
        if (!d) return

        // Send delta
        this.sendDelta(d)
    }

    previousDelta() {
        // Get idx
        const idx = this.state.current.idx !== null ? this.state.current.idx-1 : 0

        // Build delta
        return this.buildDelta(idx)
    }

    seek(idx) {
        // Reset state current
        this.resetStateCurrent()

        // Get delta
        const d = this.buildDelta(idx)
        if (!d) return

        // Send delta
        this.sendDelta(d, true)
    }

    buildDelta(idx) {
        // Check idx
        if (idx > this.state.values.length-1 || idx < 0) return null

        // Get previous values
        const previousValues = this.state.values[this.state.current.idx]

        // Update idx
        this.state.current.idx = idx

        // Get values
        const values = this.state.values[idx]
        if (!values) return null

        // Create delta
        const d = {
            at: values.at,
            stat_values: values.stats,
        }

        // Connected nodes
        values.connections.forEach(c => {
            if (!previousValues || !previousValues.connections.find(v => v.from === c.from && v.to === c.to)) {
                if (!d.connected_nodes) d.connected_nodes = []
                d.connected_nodes.push(c)
            }
        })

        // Disconnected nodes
        previousValues?.connections.forEach(c => {
            if (!values.connections.find(v => v.from === c.from && v.to === c.to)) {
                if (!d.disconnected_nodes) d.disconnected_nodes = []
                d.disconnected_nodes.push(c)
            }
        })

        // Loop through stat values
        const groups = []
        const nodes = []
        for (let statId in values.stats) {
            // Get stat
            const s = this.state.stats[statId]
            if (!s) continue

            // New stats
            if (!this.state.current.stats[s.id]) {
                if (!d.new_stats) d.new_stats = []
                d.new_stats.push(s)
                this.state.current.stats[s.id] = s
            }
            
            // Get node
            const n = this.state.nodes[s.node_id]
            if (!n) continue
            nodes.push(n)

            // Get group
            const g = this.state.groups[n.group_id]
            if (!g) continue
            groups.push(g)
        }

        // Started groups
        groups.forEach(g => {
            if (!this.state.current.groups[g.id]) {
                if (!d.started_groups) d.started_groups = []
                d.started_groups.push(g)
                this.state.current.groups[g.id] = g
            }
        })

        // Started nodes
        nodes.forEach(n => {
            if (!this.state.current.nodes[n.id]) {
                if (!d.started_nodes) d.started_nodes = []
                d.started_nodes.push(n)
                this.state.current.nodes[n.id] = n
            }
        })

        // Done groups
        const doneGroups = Object.keys(this.state.current.groups).filter(id => !groups.find(g => g.id === parseInt(id))).map(id => parseInt(id))
        if (doneGroups.length > 0) {
            doneGroups.forEach(id => delete this.state.current.groups[id])
            d.done_groups = doneGroups
        }

        // Done nodes
        const doneNodes = Object.keys(this.state.current.nodes).filter(id => !nodes.find(n => n.id === parseInt(id))).map(id => parseInt(id))
        if (doneNodes.length > 0) {
            doneNodes.forEach(id => {
                for (let k in this.state.current.stats) {
                    if (this.state.current.stats[k].node_id === id) delete this.state.current.stats[k]
                }
                delete this.state.current.nodes[id]
            })
            d.done_nodes = doneNodes
        }
        return d
    }

    send(name, payload) {
        postMessage({
            name,
            payload,
        })
    }

    sendDelta(d, clear) {
        this.send('delta', {
            clear,
            delta: d,
            idx:  this.state.current.idx,
        })
    }

    sendError(err) {
        this.send('error', err)
    }

    sendLog(...msgs) {
        this.send('log', msgs)
    }

    sendOpened(ats, catchUp) {
        this.send('opened', {
            ats,
            catchUp,
            idx:  this.state.current.idx,
        })
    }

    sendProgress(context, elapsed, label) {
        this.send('progress', {
            context,
            elapsed,
            label,
        })
    }

}

const service = new Service() 

onmessage = (e) => {
    // Switch on message name
    switch (e.data.name) {
        case 'clear':
            service.clear()
            break
        case 'next':
            service.next()
            break
        case 'open':
            service.open(e.data.payload)
            break
        case 'previous':
            service.previous()
            break
        case 'seek':
            service.seek(e.data.payload)
            break
    }
}