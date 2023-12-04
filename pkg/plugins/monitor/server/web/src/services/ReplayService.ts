import type { StoreService } from "./StoreService"
import type { LoggerService } from "./LoggerService"
import { WorkerClient } from "@/components/WorkerClient"
import type { ApiCatchUp, ApiDelta } from "@/types/api"
import type { FlowService } from "./FlowService";

enum workerMessageName {
    clear = 'clear',
    cleared = 'cleared',
    delta = 'delta',
    error = 'error',
    log = 'log',
    next = 'next',
    open = 'open',
    opened = 'opened',
    previous = 'previous',
    progress = 'progress',
    seek = 'seek',
}

interface workerDelta {
    clear: boolean,
    delta: ApiDelta,
    idx: number,
}

interface workerOpened {
    ats: number[],
    catchUp: ApiCatchUp,
    idx: number,
}

interface workerProgress {
    context: 'open',
    elapsed: number,
    label?: string,
}

export class ReplayService {

    private w: WorkerClient<workerMessageName>

    constructor(
        private flowService: FlowService,
        private loggerService: LoggerService,
        private storeService: StoreService,
    ) {
        this.w = new WorkerClient('/worker/replay.js')
        this.w.on(workerMessageName.cleared, this.onWorkerCleared)
        this.w.on<workerDelta>(workerMessageName.delta, this.onWorkerDelta)
        this.w.on(workerMessageName.error, this.onWorkerError)
        this.w.on<string[]>(workerMessageName.log, this.onWorkerLog)
        this.w.on<workerOpened>(workerMessageName.opened, this.onWorkerOpened)
        this.w.on<workerProgress>(workerMessageName.progress, this.onWorkerProgress)

        document.addEventListener('keyup', this.onKeyUp)
    }

    open(f: File) {
        // Open
        this.w.send(workerMessageName.open, f)
    }

    clear() {
        // Clear
        this.w.send(workerMessageName.clear)
    }

    private disabled(): boolean {
        return !this.storeService.get().replay.open.done
    }

    private next() {
        // Disabled
        if (this.disabled()) return

        // Next
        this.w.send(workerMessageName.next)
    }

    private previous() {
        // Disabled
        if (this.disabled()) return

        // Previous
        this.w.send(workerMessageName.previous)
    }

    seek(idx: number) {
        // Disabled
        if (this.disabled()) return

        // Seek
        this.w.send(workerMessageName.seek, idx)
    }

    private onWorkerCleared = () => {
        // Clear
        this.storeService.get().replay = {
            ats: [],
            open: {},
        }

        // Catch up
        this.flowService.catchUp()
    }

    private onWorkerDelta = (d: workerDelta) => {
        // Delta
        if (d.clear) this.flowService.clear()
        this.flowService.onDelta(d.delta)

        // Update index
        this.storeService.get().replay.currentIdx = d.idx
    }

    private onWorkerError = (err: any) => {
        // Log
        this.loggerService.error('replay:', 'worker error:', err)

        // Update
        const o = this.storeService.get().replay.open
        if (!o.done && o.progress) o.error = err
    }

    private onWorkerLog = (msgs: string[]) => {
        // Log
        this.loggerService.debug('replay:', ...msgs)
    }

    private onWorkerOpened = (o: workerOpened) => {
        // Catch up
        this.flowService.catchUp(o.catchUp)

        // Update store
        this.storeService.get().replay.ats = o.ats
        this.storeService.get().replay.currentIdx = o.idx
    }

    private onWorkerProgress = (e: workerProgress) => {
        // Update progress
        switch (e.context) {
            case 'open': {
                const o = this.storeService.get().replay.open
                if (o.progress) {
                    o.progress.elapsed = e.elapsed
                    if (e.label) o.progress.label = e.label
                } else if (e.label) {
                    o.progress = {
                        elapsed: e.elapsed,
                        label: e.label,
                    }
                }
                if (e.elapsed === 100) o.done = true
                break
            }
        }
    }

    private onKeyUp = (e: KeyboardEvent) => {
        if (e.key === 'q' && e.ctrlKey) this.previous()
        else if (e.key === 'd' && e.ctrlKey) this.next()
    }

}