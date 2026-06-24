package checks

import (
	"strings"
	"testing"
)

func TestProxmoxVMSummaryAndNodeFilter(t *testing.T) {
	resources := []proxmoxResource{
		{Type: "qemu", VMID: 100, Name: "ha", Node: "pve1", Status: "running", CPU: 0.05, Mem: 512, MaxMem: 1024, Disk: 256, MaxDisk: 1024},
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
	if !strings.Contains(details[0], "vm 100 ha @ pve1: running") {
		t.Fatalf("details[0] = %q", details[0])
	}
	if !strings.Contains(details[0], "\ncpu 5%  mem 50%") || !strings.Contains(details[0], "disk 25%") {
		t.Fatalf("details[0] = %q", details[0])
	}
}

func TestProxmoxVMSummaryIncludesLXCContainers(t *testing.T) {
	resources := []proxmoxResource{
		{Type: "qemu", VMID: 100, Name: "ha", Node: "pve1", Status: "running"},
		{Type: "lxc", VMID: 200, Name: "dns", Node: "pve1", Status: "running", CPU: 0.02, Mem: 128, MaxMem: 512},
		{Type: "lxc", VMID: 201, Name: "old", Node: "pve1", Status: "stopped"},
	}

	status, summary, details := ProxmoxVMSummary(resources)

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "VMs: 1 running, 0 stopped; LXCs: 1 running, 1 stopped" {
		t.Fatalf("summary = %q", summary)
	}
	if !containsDetail(details, "lxc 200 dns @ pve1: running") {
		t.Fatalf("details = %#v", details)
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

func TestProxmoxHealthSummaryWarnsOnHighNodeUsage(t *testing.T) {
	resources := []proxmoxResource{
		{
			Type:    "node",
			Node:    "pve1",
			Status:  "online",
			CPU:     0.91,
			MaxCPU:  8,
			Mem:     950,
			MaxMem:  1000,
			Disk:    500,
			MaxDisk: 1000,
			Uptime:  2 * 24 * 60 * 60,
		},
	}

	status, summary, details := ProxmoxHealthSummary(resources)

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "1 online, 0 offline nodes" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) < 2 || !strings.Contains(details[0], "Warnings:") {
		t.Fatalf("details missing warnings = %#v", details)
	}
	if !strings.Contains(details[0], "high cpu 91%") || !strings.Contains(details[0], "high memory 95%") {
		t.Fatalf("warnings = %q", details[0])
	}
	if !strings.Contains(details[1], "cpu 91% of 8 cores") || !strings.Contains(details[1], "uptime 2d0h") {
		t.Fatalf("node details = %q", details[1])
	}
}

func containsDetail(details []string, want string) bool {
	for _, detail := range details {
		if strings.Contains(detail, want) {
			return true
		}
	}
	return false
}
