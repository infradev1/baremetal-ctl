package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// table-driven tests
func TestCheckHealth(t *testing.T) {
	svc := &RPCSimulator{}

	tests := []struct {
		name    string
		nodeID  string
		wantErr bool
	}{
		{"healthy node", "node-1", false},
		{"unresponsive node", "node-42", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestID := uuid.New().String()
			_, err := svc.CheckHealth(context.Background(), requestID, tt.nodeID) // non-deterministic as-is
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
