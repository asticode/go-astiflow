package astiflow

const (
	DeltaStatNameHostUsage         = "astiflow.host.usage"
	DeltaStatNameIncomingByteRate  = "astiflow.incoming.byte_rate"
	DeltaStatNameIncomingRate      = "astiflow.incoming.rate"
	DeltaStatNameOutgoingByteRate  = "astiflow.outgoing.byte_rate"
	DeltaStatNameOutgoingRate      = "astiflow.outgoing.rate"
	DeltaStatNameProcessedByteRate = "astiflow.processed.byte_rate"
	DeltaStatNameProcessedRate     = "astiflow.processed.rate"
)

type DeltaStatHostUsageValue struct {
	CPU    DeltaStatHostCPUUsageValue    `json:"cpu"`
	Memory DeltaStatHostMemoryUsageValue `json:"memory"`
}

type DeltaStatHostCPUUsageValue struct {
	Individual []float64 `json:"individual"`
	Process    *float64  `json:"process,omitempty"`
	Total      float64   `json:"total"`
}

type DeltaStatHostMemoryUsageValue struct {
	Resident uint64 `json:"resident"`
	Total    uint64 `json:"total"`
	Used     uint64 `json:"used"`
	Virtual  uint64 `json:"virtual"`
}
