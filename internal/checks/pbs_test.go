package checks

import (
	"strings"
	"testing"
)

func TestPBSSummaryWarnsAbove80Percent(t *testing.T) {
	statuses := []pbsDatastoreStatus{
		{Store: "main", Used: 85, Total: 100},
	}

	status, summary, details := PBSSummary(statuses)

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "1 datastores, highest 85% used" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}
}

func TestPBSSummaryErrorsAbove90Percent(t *testing.T) {
	statuses := []pbsDatastoreStatus{
		{Store: "main", Used: 91, Total: 100},
	}

	status, _, _ := PBSSummary(statuses)

	if status != StatusError {
		t.Fatalf("status = %q, want %q", status, StatusError)
	}
}

func TestPBSTasksSummary(t *testing.T) {
	tasks := []pbsTask{
		{WorkerType: "backup", WorkerID: "vm/100", Status: "OK", StartTime: 1779820000},
		{WorkerType: "syncjob", WorkerID: "remote", Status: "TASK ERROR: failed"},
		{WorkerType: "garbage_collection"},
	}

	status, summary, details := PBSTasksSummary(tasks)

	if status != StatusError {
		t.Fatalf("status = %q, want %q", status, StatusError)
	}
	if summary != "24h tasks: 1 ok, 1 running, 1 failed" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 3 {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(details[0], "backup") || !strings.Contains(details[0], "vm/100") {
		t.Fatalf("details[0] = %q", details[0])
	}
}

func TestPBSTasksSummaryEmpty(t *testing.T) {
	status, summary, details := PBSTasksSummary(nil)

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "no visible tasks in last 24h" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}
}
