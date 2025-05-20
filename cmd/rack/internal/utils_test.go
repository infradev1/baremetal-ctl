package internal

import (
	"context"
	"sync"
	"testing"
)

func TestSubmitJobs(t *testing.T) {
	rackPowerBudget := 350
	wg := &sync.WaitGroup{}
	totalPower := &RackPower{}
	nodes, err := CreateNodes(context.Background(), wg, totalPower, make(chan<- Result), 2, 100, 200, rackPowerBudget, 2, 10)
	if err != nil {
		t.Fail() // we should not be exceeding the budget yet
	}
	// 350 - 200 so far = 150 budget left
	if err := SubmitJobs(nodes, wg, 2, 100, rackPowerBudget, totalPower); err == nil {
		t.Fail() // we should be getting an error for exceeding the rack power budget
	}
}
