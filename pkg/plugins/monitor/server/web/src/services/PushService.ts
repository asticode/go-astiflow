import { EventManager, type Remover } from "@/components/EventManager"
import type { ApiDelta } from "@/types/api"
import type { ConfigService } from "./ConfigService"
import type { LoggerService } from "./LoggerService"
import { WebSocketClient } from "@/components/WebSocketClient"
import type { StoreService } from "./StoreService"

export type Event = {
    name: EventName,
    payload?: any,
}

export enum EventName {
    delta = 'delta',
    ping = 'ping',
}

export interface Pusher {
    onEvent(f: (e: Event) => void): void
}

class DisabledError {}

export class PushService {

    private m: EventManager

    constructor(
        private configService: ConfigService,
        private loggerService: LoggerService,
        private storeService: StoreService
    ) {
        this.m = new EventManager()
        this.pusher()
            .then(p => p.onEvent(this.onEvent))
            .catch(err => { if (!(err instanceof DisabledError)) this.loggerService.error(err) })
    }

    private async pusher(): Promise<Pusher> {
        const c = await this.configService.get()
        if (!c.push.url) throw new DisabledError()
        let url = c.push.url
        const queryParams = new URLSearchParams()
        for (const k in c.push.queryParams) {
            queryParams.set(k, c.push.queryParams[k])
          }
        if (url.startsWith('/')) url = 'ws://' + location.host + url + (queryParams.size > 0 ? '?' + queryParams.toString() : '')
        if (url.startsWith('ws://') || url.startsWith('wss://')) return new WebSocketClient(this.loggerService, url)
        else throw `Creating pusher for url "${url}" failed: protocol not handled`
    }

    private onEvent = (e: Event) => {
        if (this.storeService.get().replay.open.done) return
        this.m.emit(e.name, e.payload)
    }

    onDelta(h: (p: ApiDelta) => void): Remover {
        return this.m.on(EventName.delta, h)
    }

}