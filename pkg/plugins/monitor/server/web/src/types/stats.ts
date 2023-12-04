export interface HostUsageStatValue {
    cpu: HostCPUUsageStatValue
    memory: HostMemoryUsageStatValue
}

interface HostCPUUsageStatValue {
    individual: number[]
    process?: number
    total: number
}

interface HostMemoryUsageStatValue {
    resident: number
    total: number
    used: number
    virtual: number
}