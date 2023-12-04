export interface Config {
    api: {
        headers?: Record<string, string>,
        queryParams?: Record<string, string>,
        url?: string,
    }
    push: {
        queryParams?: Record<string, string>,
        url?: string,
    }
}