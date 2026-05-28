package checks

import (
	"strings"
	"testing"
)

func TestProxmoxVMSummaryAndNodeFilter(t *testing.T) {
	resources := []proxmoxResource{
		{Type: "qemu", VMID: 100, Name: "ha", Node: "pve1", Status: "running", CPU: 0.05, Mem: 512, MaxMem: 1024},
		{Type: "qemu", VMID: 101, Name: "test", Node: "pve2", Status: "stopped"},
	}

	filtered := filterProxmoxResources(resources, []string{"pve1"})
	status, summary, details := ProxmoxVMSummary(filtered)

	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if summary != "1 running, 0 stopped VMs" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(details[0], "\ncpu 5%  mem 50%") {
		t.Fatalf("details[0] = %q", details[0])
	}
}

func TestProxmoxHealthSummaryOfflineNode(t *testing.T) {
	resources := []proxmoxResource{
		{Type: "node", Node: "pve1", Status: "online"},
		{Type: "node", Node: "pve2", Status: "offline"},
	}

	status, summary, _ := ProxmoxHealthSummary(resources)

	if status != StatusError {
		t.Fatalf("status = %q, want %q", status, StatusError)
	}
	if summary != "1 online, 1 offline nodes" {
		t.Fatalf("summary = %q", summary)
	}
}
