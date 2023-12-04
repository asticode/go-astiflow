import type { HttpClient } from "@/components/HttpClient";
import type { Config } from "@/types/config";

export class ConfigService {

    private c: Config | undefined

    constructor(
        private httpClient: HttpClient
    ) {}

    async get(): Promise<Config> {
        if (!this.c) {
            // Create context
            const ctx = new URLSearchParams(location.search)

            // Get config
            this.c = await this.httpClient.do<void, Config>({ url: '/config.json' })

            // Apply context
            if (this.c.api.headers) this.applyContextToRecord(ctx, this.c.api.headers)
            if (this.c.api.queryParams) this.applyContextToRecord(ctx, this.c.api.queryParams)
            if (this.c.api.url) this.c.api.url = this.applyContext(ctx, this.c.api.url)
            if (this.c.push.queryParams) this.applyContextToRecord(ctx, this.c.push.queryParams)
            if (this.c.push.url) this.c.push.url = this.applyContext(ctx, this.c.push.url)
        }
        return this.c
    }

    private applyContextToRecord(ctx: URLSearchParams, m: Record<string, string>): Record<string, string> {
        for (const k in m) { m[k] = this.applyContext(ctx, m[k]) }
        return m
    }

    private applyContext(ctx: URLSearchParams, v: string): string {
        ctx.forEach((tv: string, tk: string) => v = v.replace(`{{ ${tk} }}`, tv))
        return v
    }

}