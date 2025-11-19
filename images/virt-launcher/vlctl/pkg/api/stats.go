/*
Copyright The KubeVirt Authors.
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/v1.6.2/pkg/virt-launcher/virtwrap/stats/types.go
*/

package api

type DomainStats struct {
	// the following aren't really needed for stats, but it's practical to report
	// OTOH, the whole "Domain" is too much data to be unconditionally reported
	Name string
	UUID string
	// omitted from libvirt-go: Domain
	// omitted from libvirt-go: State
	Cpu *DomainStatsCPU
	// new, see below
	Memory *DomainStatsMemory
	// omitted from libvirt-go: DomainJobInfo
	MigrateDomainJobInfo *DomainJobInfo
	// omitted from libvirt-go: Balloon
	Vcpu  []DomainStatsVcpu
	Net   []DomainStatsNet
	Block []DomainStatsBlock
	// omitted from libvirt-go: Perf
	// extra stats
	CPUMapSet bool
	CPUMap    [][]bool
	NrVirtCpu uint
	DirtyRate *DomainStatsDirtyRate
}

type DomainStatsCPU struct {
	TimeSet   bool
	Time      uint64
	UserSet   bool
	User      uint64
	SystemSet bool
	System    uint64
}

type DomainStatsVcpu struct {
	StateSet bool
	State    int // VcpuState
	TimeSet  bool
	Time     uint64
	WaitSet  bool
	Wait     uint64
	DelaySet bool
	Delay    uint64
}

type DomainStatsNet struct {
	NameSet    bool
	Name       string
	AliasSet   bool
	Alias      string
	RxBytesSet bool
	RxBytes    uint64
	RxPktsSet  bool
	RxPkts     uint64
	RxErrsSet  bool
	RxErrs     uint64
	RxDropSet  bool
	RxDrop     uint64
	TxBytesSet bool
	TxBytes    uint64
	TxPktsSet  bool
	TxPkts     uint64
	TxErrsSet  bool
	TxErrs     uint64
	TxDropSet  bool
	TxDrop     uint64
}

type DomainStatsBlock struct {
	NameSet         bool
	Name            string
	Alias           string
	BackingIndexSet bool
	BackingIndex    uint
	PathSet         bool
	Path            string
	RdReqsSet       bool
	RdReqs          uint64
	RdBytesSet      bool
	RdBytes         uint64
	RdTimesSet      bool
	RdTimes         uint64
	WrReqsSet       bool
	WrReqs          uint64
	WrBytesSet      bool
	WrBytes         uint64
	WrTimesSet      bool
	WrTimes         uint64
	FlReqsSet       bool
	FlReqs          uint64
	FlTimesSet      bool
	FlTimes         uint64
	ErrorsSet       bool
	Errors          uint64
	AllocationSet   bool
	Allocation      uint64
	CapacitySet     bool
	Capacity        uint64
	PhysicalSet     bool
	Physical        uint64
}

// mimic existing structs, but data is taken from
// DomainMemoryStat
type DomainStatsMemory struct {
	UnusedSet        bool
	Unused           uint64
	CachedSet        bool
	Cached           uint64
	AvailableSet     bool
	Available        uint64
	ActualBalloonSet bool
	ActualBalloon    uint64
	RSSSet           bool
	RSS              uint64
	SwapInSet        bool
	SwapIn           uint64
	SwapOutSet       bool
	SwapOut          uint64
	MajorFaultSet    bool
	MajorFault       uint64
	MinorFaultSet    bool
	MinorFault       uint64
	UsableSet        bool
	Usable           uint64
	TotalSet         bool
	Total            uint64
}

// mimic existing structs, but data is taken from
// DomainJobInfo
type DomainJobInfo struct {
	DataTotalSet     bool
	DataTotal        uint64
	DataProcessedSet bool
	DataProcessed    uint64
	MemoryBpsSet     bool
	MemoryBps        uint64
	DataRemainingSet bool
	DataRemaining    uint64
	MemDirtyRateSet  bool
	MemDirtyRate     uint64
}

type DomainStatsDirtyRate struct {
	CalcStatusSet         bool
	CalcStatus            int
	CalcStartTimeSet      bool
	CalcStartTime         int64
	CalcPeriodSet         bool
	CalcPeriod            int
	MegabytesPerSecondSet bool
	MegabytesPerSecond    int64
}
