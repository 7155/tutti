package main

import (
	"context"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	replaydata "github.com/tutti-os/tutti/services/tuttid/data/agentsessionreplay"
	workspacedata "github.com/tutti-os/tutti/services/tuttid/data/workspace"
	replayservice "github.com/tutti-os/tutti/services/tuttid/service/agentsessionreplay"
)

func prepareReplaySemanticRuntime(
	ctx context.Context,
	store *workspacedata.SQLiteStore,
	host *agenthost.Host,
	registrations []agentSessionReplayRegistration,
) (*replayservice.SemanticRuntime, error) {
	artifactDirectories := make(map[string]string, len(registrations))
	semanticRegistrations := make(
		[]replayservice.SemanticRegistration,
		0,
		len(registrations),
	)
	for _, registration := range registrations {
		artifactDirectories[registration.CassetteID] = registration.ArtifactDirectory
		semanticRegistrations = append(
			semanticRegistrations,
			replayservice.SemanticRegistration{
				CassetteID:    registration.CassetteID,
				RootSessionID: registration.RootAgentSessionID,
				WorkspaceID:   registration.WorkspaceID,
			},
		)
	}
	reader, err := replaydata.NewSemanticCassetteReader(artifactDirectories)
	if err != nil {
		return nil, err
	}
	return replayservice.PrepareSemanticRuntime(
		ctx,
		store,
		host,
		reader,
		semanticRegistrations,
	)
}
