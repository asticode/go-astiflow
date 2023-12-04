import { EventManager } from "@/components/EventManager"
import type { LoggerService } from "../services/LoggerService"
import { EventName, type Event, type Pusher } from "../services/PushService"

enum eventName {
    event = 'event',
}

export class WebSocketClient implements Pusher {

    private m: EventManager
    private pingInterval?: number
    private ws!: WebSocket

    constructor(
        private loggerService: LoggerService,
        private url: string,
    ) {
        this.m = new EventManager()
        this.open()
    }

    private open = () => {
        this.loggerService.debug('ws:', 'opening')
        this.ws = new WebSocket(this.url)
        this.ws.addEventListener('close', this.onClose)
        this.ws.addEventListener('error', this.onError)
        this.ws.addEventListener('message', this.onMessage)
        this.ws.addEventListener('open', this.onOpen)
    }

    private onClose = () => {
        this.loggerService.debug('ws:', 'closed')
        if (this.pingInterval) {
            clearInterval(this.pingInterval)
            this.pingInterval = undefined
        }
        this.loggerService.debug('ws:', 'retrying in 2s')
        setTimeout(this.open, 2e3)
    }

    private onError = (err: any) => {
        this.loggerService.debug('ws:', 'error', err)
    }

    private onOpen = () => {
        this.loggerService.debug('ws:', 'opened')
        this.pingInterval = setInterval(() => {
            this.write({ name: EventName.ping })
        }, 55e3)
    }

    private onMessage = (i: MessageEvent<string>) => {
        // Parse data
        let e: Event
        try {
           e = JSON.parse(i.data) 
        } catch (err) {
            this.onError(err)
            return
        }
        
        // Emit
        this.m.emit(eventName.event, e)
    }

    private write = (e: Event) => {
        this.ws.send(JSON.stringify(e))
    }

    onEvent(f: (e: Event) => void): void {
        this.m.on(eventName.event, f)
    }

}