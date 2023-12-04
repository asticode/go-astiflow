import { EventManager } from "./EventManager";

export class WorkerClient<MessageName> {

    private e: EventManager
    private w: Worker

    constructor(url: string) {
        this.e = new EventManager()
        this.w = new Worker(url)
        this.w.addEventListener('error', this.onError)
        this.w.addEventListener('message', this.onMessage)
    }

    send(n: MessageName, p?: any) {
        this.w.postMessage({
            name: n,
            payload: p,
        })
    }

    on<T>(n: MessageName, f: (p: T) => void) {
        this.e.on(String(n), f)
    }

    private onError = (e: ErrorEvent) => {
        this.e.emit('error', e.error)
    }

    private onMessage = (e: MessageEvent) => {
        this.e.emit(e.data.name, e.data.payload)
    }

}