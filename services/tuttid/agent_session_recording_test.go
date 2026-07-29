package main

import (
	"context"
	"testing"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	replay "github.com/tutti-os/tutti/packages/agent/session-replay"
)

type countingAgentCommitObserver struct {
	calls int
}

func (o *countingAgentCommitObserver) ObserveCommitted(context.Context, agenthost.CommittedDelta) error {
	o.calls++
	return nil
}

type countingAgentProviderObservationObserver struct {
	calls int
}

func (o *countingAgentProviderObservationObserver) ObserveProviderObservations(
	context.Context,
	string,
	string,
	[]replay.ProviderObservationBatch,
) error {
	o.calls++
	return nil
}

func TestAgentObserverFanoutsDeliverOnceToEveryObserver(t *testing.T) {
	t.Parallel()

	projection := &countingAgentCommitObserver{}
	recordingCommit := &countingAgentCommitObserver{}
	commitObservers := newAgentCommitObserverRelay(projection)
	commitObservers.Add(recordingCommit)
	if err := commitObservers.ObserveCommitted(
		context.Background(),
		agenthost.CommittedDelta{},
	); err != nil {
		t.Fatalf("observe committed: %v", err)
	}
	if projection.calls != 1 || recordingCommit.calls != 1 {
		t.Fatalf(
			"commit fanout calls = projection:%d recording:%d, want 1 each",
			projection.calls,
			recordingCommit.calls,
		)
	}

	recordingProvider := &countingAgentProviderObservationObserver{}
	semanticRuntime := &countingAgentProviderObservationObserver{}
	if err := (agentProviderObservationObservers{
		recordingProvider,
		semanticRuntime,
	}).ObserveProviderObservations(
		context.Background(),
		"workspace-1",
		"session-1",
		nil,
	); err != nil {
		t.Fatalf("observe provider observations: %v", err)
	}
	if recordingProvider.calls != 1 || semanticRuntime.calls != 1 {
		t.Fatalf(
			"provider fanout calls = recording:%d semantic:%d, want 1 each",
			recordingProvider.calls,
			semanticRuntime.calls,
		)
	}
}
