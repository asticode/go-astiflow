type Handler<P> = (p: P) => void

export type Remover = () => void

export class EventManager {
    
    private hs: Record<string, Handler<any>[]> = {}

    on = (name: string, h: Handler<any>): Remover => {
        // Make sure name exists
        if (!(name in this.hs)) this.hs[name] = []

        // Add handler
        this.hs[name].push(h)
        return () => { this.off(name, h) }
    }

    off = (name: string, h: Handler<any>) => {
        // Make sure name exists
        if (!(name in this.hs)) return

        // Remove handler
        this.hs[name] = this.hs[name].filter(v => v !== h)
    }

    emit = (name: string, p: any) => {
        // Make sure name exists
        if (!(name in this.hs)) return
        
        // Loop through handlers
        this.hs[name].forEach(h => h(p))
    }

}