package main

import "testing"

func TestLog_Update(t *testing.T) {
	log := NewLog()

	log.Update("gpu-node-1", "[INFO]")
	log.Update("gpu-node-1", "[INFO]")
	log.Update("gpu-node-1", "[ERROR]")

	counts := log.nodeInfo["gpu-node-1"]
	if counts["[INFO]"] != 2 {
		t.Errorf("expected 2 INFO, got %d", counts["[INFO]"])
	}
	if counts["[ERROR]"] != 1 {
		t.Errorf("expected 1 ERROR, got %d", counts["[ERROR]"])
	}
}
