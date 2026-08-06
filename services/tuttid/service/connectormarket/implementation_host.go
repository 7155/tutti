//revive:disable:file-length-limit

package connectormarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/tutti-os/tutti/packages/agent/daemon/runtime"
	market "github.com/tutti-os/tutti/packages/connector/host"
	connectorruntime "github.com/tutti-os/tutti/packages/connector/runtime"
	cliservice "github.com/tutti-os/tutti/services/tuttid/service/cli"
	mcpservice "github.com/tutti-os/tutti/services/tuttid/service/mcp"
)

type PreparedArtifactResolver interface {
	ResolvePrepared(context.Context, market.Release) (market.PreparedArtifactReceipt, error)
}

type ConnectorRuntimeResolver = connectorruntime.ConnectorRuntimeResolver

type ImplementationHostConfig struct {
	Artifacts              PreparedArtifactResolver
	CLIInstallations       market.CLIInstallationManager
	Runtimes               ConnectorRuntimeResolver
	Processes              agentruntime.ProcessTransport
	Commands               *ConnectorCommandRegistry
	StateRoot              string
	MCPStartupTimeout      time.Duration
	RemoteHTTPClient       *http.Client
	AuthorizeRemoteRequest mcpservice.RequestAuthorizer
}

type ImplementationHost struct {
	artifacts              PreparedArtifactResolver
	planner                *connectorruntime.ManagedRoutePlanner
	processes              agentruntime.ProcessTransport
	mcpStartupTimeout      time.Duration
	remoteHTTPClient       *http.Client
	authorizeRemoteRequest mcpservice.RequestAuthorizer
	routes                 *connectorruntime.RouteTable
	snapshots              *connectorruntime.ExecutionSnapshotter
}

type connectorRoute struct {
	id            string
	connectionID  string
	connectorKey  string
	releaseDigest string
	generation    market.HostGeneration
	capabilities  map[string]connectorCommand
	closeMu       sync.Mutex
	mcpClient     *mcpservice.StdioClient
	remoteMCP     *mcpservice.StreamableHTTPClient
	executionRoot string
	installedRoot string
	displayName   string
	description   string
	processes     *connectorruntime.ProcessGroup
	snapshots     *connectorruntime.ExecutionSnapshotter
}

type connectorCommand struct {
	capability cliservice.Capability
	invoke     func(context.Context, cliservice.InvokeRequest) (cliservice.CommandOutput, error)
}

func NewImplementationHost(config ImplementationHostConfig) (*ImplementationHost, error) {
	if config.Artifacts == nil || config.Runtimes == nil || config.Processes == nil || config.Commands == nil {
		return nil, errors.New("connector implementation host dependencies are required")
	}
	if !filepath.IsAbs(strings.TrimSpace(config.StateRoot)) {
		return nil, errors.New("connector implementation state root must be absolute")
	}
	if config.MCPStartupTimeout <= 0 {
		config.MCPStartupTimeout = 15 * time.Second
	}
	snapshots, err := connectorruntime.NewExecutionSnapshotter(config.StateRoot)
	if err != nil {
		return nil, err
	}
	routes := connectorruntime.NewRouteTable()
	planner, err := connectorruntime.NewManagedRoutePlanner(connectorruntime.ManagedRoutePlannerConfig{
		StateRoot: config.StateRoot, Runtimes: config.Runtimes, CLIInstallations: config.CLIInstallations,
	})
	if err != nil {
		return nil, err
	}
	config.Commands.attach(routes)
	return &ImplementationHost{artifacts: config.Artifacts, planner: planner, processes: config.Processes,
		mcpStartupTimeout: config.MCPStartupTimeout,
		remoteHTTPClient:  config.RemoteHTTPClient, authorizeRemoteRequest: config.AuthorizeRemoteRequest,
		routes: routes, snapshots: snapshots}, nil
}

func (host *ImplementationHost) Reconcile(ctx context.Context, request market.RuntimeReconcileRequest) (market.RuntimeReceipt, error) {
	if host == nil || !hostIdentityPattern.MatchString(request.ConnectionID) || !hostIdentityPattern.MatchString(request.Connector.Key) || request.Generation.BootEpoch == "" || request.Generation.Generation == 0 {
		return market.RuntimeReceipt{}, errors.New("connector runtime reconcile identity is invalid")
	}
	if err := market.ValidateRuntimeReleaseShape(request.Connector.Release); err != nil {
		return market.RuntimeReceipt{}, err
	}
	key := connectorRouteKey(request.ConnectionID, request.Connector.Key)
	if !request.Enabled {
		if err := host.removeRoute(key, request.Generation, "", time.Time{}); err != nil {
			return market.RuntimeReceipt{}, err
		}
		return market.RuntimeReceipt{OperationID: request.OperationID, ConnectionID: request.ConnectionID,
			ConnectorKey: request.Connector.Key, ReleaseDigest: request.Connector.Installation.InstalledReleaseDigest, Generation: request.Generation}, nil
	}
	implementation := request.Connector.Release.Manifest.Implementation
	if implementation.Kind == market.ImplementationKindManagedStdio &&
		(request.Connector.Authorization.State != market.AuthorizationStateNotRequired || request.Connector.Release.Manifest.AuthorizationKind != "none") {
		return market.RuntimeReceipt{}, errors.New("connector credential broker is not available for authorized connectors")
	}
	if request.Connector.Installation.State != market.InstallationStateInstalled ||
		request.Connector.Installation.InstalledReleaseDigest != request.Connector.Release.ReleaseDigest {
		return market.RuntimeReceipt{}, errors.New("connector installed release is not active")
	}
	var (
		route *connectorRoute
		err   error
	)
	if implementation.Kind == market.ImplementationKindRemoteStreamableHTTP {
		route, err = host.buildRemoteRoute(ctx, request)
	} else {
		prepared, resolveErr := host.artifacts.ResolvePrepared(ctx, request.Connector.Release)
		if resolveErr != nil {
			return market.RuntimeReceipt{}, fmt.Errorf("resolve prepared connector artifact: %w", resolveErr)
		}
		installedRoot := prepared.PreparedPath
		executionRoot, snapshotErr := host.snapshots.Create(prepared)
		if snapshotErr != nil {
			return market.RuntimeReceipt{}, fmt.Errorf("create connector execution snapshot: %w", snapshotErr)
		}
		prepared.PreparedPath = executionRoot
		route, err = host.buildManagedRoute(ctx, request, prepared)
		if err != nil {
			_ = host.snapshots.Remove(executionRoot)
			return market.RuntimeReceipt{}, err
		}
		route.executionRoot = executionRoot
		route.installedRoot = installedRoot
	}
	if err != nil {
		return market.RuntimeReceipt{}, err
	}
	route.displayName = request.Connector.Release.Manifest.DisplayName
	route.description = request.Connector.Release.Manifest.Description
	route.snapshots = host.snapshots
	if err := host.commitRoute(key, route); err != nil {
		_ = route.Close(time.Now().Add(3 * time.Second))
		return market.RuntimeReceipt{}, err
	}
	if route.mcpClient != nil {
		go host.monitorMCPRoute(route, route.mcpClient)
	}
	routeIDs := make([]string, 0, len(route.capabilities))
	for routeID := range route.capabilities {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Strings(routeIDs)
	return market.RuntimeReceipt{OperationID: request.OperationID, ConnectionID: request.ConnectionID,
		ConnectorKey: request.Connector.Key, ReleaseDigest: route.releaseDigest, Generation: request.Generation, RouteIDs: routeIDs}, nil
}

func (host *ImplementationHost) Close() error {
	if host == nil {
		return nil
	}
	return host.routes.Close(time.Now().Add(3 * time.Second))
}

// SetCapabilityPublication gates discovery and invocation without preventing
// bootstrap from staging validated MCP processes and CLI routes. Enabling is a
// single registry state transition after every durable binding reconciles.
func (host *ImplementationHost) SetCapabilityPublication(enabled bool) {
	if host != nil {
		host.routes.SetPublished(enabled)
	}
}

// FenceAll deactivates every staged or published route, including a route whose
// operation failed before its durable binding was committed.
func (host *ImplementationHost) FenceAll(_ context.Context, deadline time.Time) error {
	if host == nil {
		return nil
	}
	return host.routes.FenceAll(deadline)
}

func (host *ImplementationHost) FailClosed(ctx context.Context, deadline time.Time) error {
	if host == nil {
		return nil
	}
	host.SetCapabilityPublication(false)
	return host.FenceAll(ctx, deadline)
}

func (host *ImplementationHost) DeactivateRuntime(ctx context.Context, request market.RuntimeDeactivationRequest) error {
	if host == nil {
		return errors.New("connector implementation host is unavailable")
	}
	if !request.Deadline.IsZero() && time.Now().After(request.Deadline) {
		return context.DeadlineExceeded
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return host.removeRoute(connectorRouteKey(request.ConnectionID, request.ConnectorKey), request.Generation, request.ReleaseDigest, request.Deadline)
}

func (host *ImplementationHost) buildManagedRoute(ctx context.Context, request market.RuntimeReconcileRequest, prepared market.PreparedArtifactReceipt) (*connectorRoute, error) {
	plan, err := host.planner.Build(ctx, request, prepared)
	if err != nil {
		return nil, err
	}
	route := newConnectorRoute(request)
	if plan.Managed.MCP != nil {
		if err := host.attachMCP(ctx, route, plan.Managed, prepared, plan.Executable, plan.SandboxPolicy); err != nil {
			_ = route.close(time.Now().Add(3 * time.Second))
			return nil, err
		}
	}
	if plan.Managed.CLI != nil {
		if err := host.attachCLI(route, plan.Managed, prepared, plan.InstalledCLI, plan.Executable, plan.SandboxPolicy); err != nil {
			_ = route.close(time.Now().Add(3 * time.Second))
			return nil, err
		}
	}
	if len(route.capabilities) == 0 {
		_ = route.close(time.Now().Add(3 * time.Second))
		return nil, errors.New("connector implementation exposed no MCP tools or CLI commands")
	}
	return route, nil
}

func newConnectorRoute(request market.RuntimeReconcileRequest) *connectorRoute {
	return &connectorRoute{id: connectorRouteKey(request.ConnectionID, request.Connector.Key), connectionID: request.ConnectionID,
		connectorKey: request.Connector.Key, releaseDigest: request.Connector.Release.ReleaseDigest,
		generation: request.Generation, capabilities: make(map[string]connectorCommand),
		processes: connectorruntime.NewProcessGroup()}
}

func (host *ImplementationHost) buildRemoteRoute(ctx context.Context, request market.RuntimeReconcileRequest) (*connectorRoute, error) {
	remote := request.Connector.Release.Manifest.Implementation.RemoteStreamableHTTP
	if remote == nil {
		return nil, errors.New("remote_streamable_http connector config is unavailable")
	}
	var authorizer mcpservice.RequestAuthorizer
	switch remote.Authentication.Type {
	case "none":
	case "host_session":
		if host.authorizeRemoteRequest == nil {
			return nil, errors.New("remote MCP host-session authentication is unavailable")
		}
		authorizer = host.authorizeRemoteRequest
	default:
		return nil, errors.New("remote MCP authentication type is unsupported")
	}
	client, err := mcpservice.NewStreamableHTTPClient(mcpservice.StreamableHTTPClientConfig{
		Endpoint: remote.Endpoint, AllowedHosts: remote.AllowedHosts, HTTPClient: host.remoteHTTPClient,
		AuthorizeRequest: authorizer, Timeout: time.Duration(remote.Limits.TimeoutMS) * time.Millisecond,
		MaxResponseBytes: remote.Limits.MaxResponseBytes,
	})
	if err != nil {
		return nil, err
	}
	route := newConnectorRoute(request)
	closeClient := func() { _ = client.Close(context.Background()) }
	if _, err := client.Call(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "tuttid-connector-host", "version": "1"}}); err != nil {
		closeClient()
		return nil, fmt.Errorf("initialize remote connector MCP: %w", err)
	}
	if err := client.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		closeClient()
		return nil, err
	}
	tools, err := listConnectorMCPTools(ctx, client)
	if err != nil {
		closeClient()
		return nil, err
	}
	if err := host.registerMCPTools(route, client, tools); err != nil {
		closeClient()
		return nil, err
	}
	route.remoteMCP = client
	return route, nil
}

func (host *ImplementationHost) attachMCP(ctx context.Context, route *connectorRoute, managed *market.ManagedStdioImplementation, prepared market.PreparedArtifactReceipt, executable connectorruntime.ConnectorExecutable, sandbox *agentruntime.ConnectorSandboxPolicy) error {
	entrypoint, err := connectorruntime.PreparedEntrypoint(prepared.PreparedPath, managed.MCP.Entrypoint)
	if err != nil {
		return err
	}
	startupContext, cancelStartup := context.WithTimeout(ctx, host.mcpStartupTimeout)
	defer cancelStartup()
	startContext, processID, accepted := route.beginProcess(context.Background())
	if !accepted {
		return errors.New("connector MCP route was fenced during startup")
	}
	connection, err := host.awaitProcessStart(startupContext, route, processID, startContext,
		connectorruntime.ConnectorProcessSpec(route.connectionID, route.connectorKey, managed.Runtime.Language, executable, prepared.PreparedPath,
			append([]string{entrypoint}, managed.MCP.Arguments...), sandbox))
	if err != nil {
		return fmt.Errorf("start connector MCP process: %w", err)
	}
	release := func() { route.releaseProcess(processID, connection) }
	client, err := mcpservice.NewStdioClient(mcpservice.StdioClientConfig{Connection: connection, ProcessName: route.connectorKey + " MCP"})
	if err != nil {
		release()
		return err
	}
	if _, err := client.Call(startupContext, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "tuttid-connector-host", "version": "1"}}); err != nil {
		release()
		return fmt.Errorf("initialize connector MCP process: %w", err)
	}
	if err := client.Notify("notifications/initialized", map[string]any{}); err != nil {
		release()
		return err
	}
	tools, err := listConnectorMCPTools(startupContext, client)
	if err != nil {
		release()
		return err
	}
	if len(tools) == 0 {
		release()
		return errors.New("connector MCP tools/list response is invalid")
	}
	if err := host.registerMCPTools(route, client, tools); err != nil {
		release()
		return err
	}
	route.mcpClient = client
	return nil
}

type connectorMCPCaller interface {
	Call(context.Context, string, any) (json.RawMessage, error)
}

func (host *ImplementationHost) registerMCPTools(route *connectorRoute, client connectorMCPCaller, tools []connectorMCPTool) error {
	if len(tools) == 0 {
		return errors.New("connector MCP tools/list response is invalid")
	}
	for _, tool := range tools {
		tool := tool
		commandID, err := connectorCapabilityID(route.connectorKey, "mcp", tool.Name)
		if err != nil || tool.InputSchema == nil || tool.InputSchema["type"] != "object" || cliservice.ValidateCapabilityInputSchema(tool.InputSchema) != nil {
			return errors.New("connector MCP tool contract is invalid")
		}
		if _, duplicate := route.capabilities[commandID]; duplicate {
			return errors.New("connector MCP tool capability id is duplicated")
		}
		route.capabilities[commandID] = connectorCommand{capability: connectorCapability(commandID, route.connectorKey, tool.Name, tool.Description, tool.InputSchema),
			invoke: func(callCtx context.Context, request cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
				if !host.routeCurrent(route) {
					return cliservice.CommandOutput{}, cliservice.ErrServiceUnavailable
				}
				result, err := client.Call(callCtx, "tools/call", map[string]any{"name": tool.Name, "arguments": request.Input})
				if err != nil {
					return cliservice.CommandOutput{}, cliservice.ServiceUnavailableError("connector MCP tool failed", err)
				}
				return jsonCommandOutput(result)
			}}
	}
	return nil
}

type connectorMCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func listConnectorMCPTools(ctx context.Context, client connectorMCPCaller) ([]connectorMCPTool, error) {
	const maxPages = 64
	const maxTools = 512
	result := make([]connectorMCPTool, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < maxPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := client.Call(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("list connector MCP tools: %w", err)
		}
		var listing struct {
			Tools      []connectorMCPTool `json:"tools"`
			NextCursor string             `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &listing); err != nil {
			return nil, errors.New("connector MCP tools/list response is invalid")
		}
		result = append(result, listing.Tools...)
		if len(result) > maxTools {
			return nil, errors.New("connector MCP tools/list exceeds tool limit")
		}
		next := strings.TrimSpace(listing.NextCursor)
		if next == "" {
			return result, nil
		}
		if _, duplicate := seen[next]; duplicate {
			return nil, errors.New("connector MCP tools/list cursor repeated")
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, errors.New("connector MCP tools/list exceeds page limit")
}

func (host *ImplementationHost) monitorMCPRoute(route *connectorRoute, client *mcpservice.StdioClient) {
	<-client.Done()
	_ = host.retireExactRoute(route, time.Now().Add(3*time.Second))
}

func (host *ImplementationHost) attachCLI(route *connectorRoute, managed *market.ManagedStdioImplementation,
	prepared market.PreparedArtifactReceipt, installed *market.CLIInstallationReceipt,
	executable connectorruntime.ConnectorExecutable, sandbox *agentruntime.ConnectorSandboxPolicy) error {
	entrypointRoot, entrypointRelative := prepared.PreparedPath, managed.CLI.Entrypoint
	if installed != nil {
		entrypointRoot, entrypointRelative = installed.InstallRoot, installed.Entrypoint
	}
	entrypoint, err := connectorruntime.PreparedEntrypoint(entrypointRoot, entrypointRelative)
	if err != nil {
		return err
	}
	launchArguments := []string{entrypoint}
	launchExecutable := executable
	if installed != nil && installed.LaunchKind == "native" {
		launchArguments = nil
		launchExecutable = connectorruntime.ConnectorExecutable{Path: entrypoint, SHA256: installed.EntrypointSHA256,
			SizeBytes: installed.EntrypointSize}
	}
	if len(managed.CLI.Commands) == 0 {
		return host.attachGenericCLI(route, managed, prepared, launchArguments, launchExecutable, sandbox)
	}
	for _, manifestCommand := range managed.CLI.Commands {
		manifestCommand := manifestCommand
		commandID, err := connectorCapabilityID(route.connectorKey, "cli", manifestCommand.Name)
		if err != nil {
			return err
		}
		if _, duplicate := route.capabilities[commandID]; duplicate {
			return errors.New("connector CLI capability id is duplicated")
		}
		if err := cliservice.ValidateCapabilityInputSchema(manifestCommand.InputSchema); err != nil {
			return fmt.Errorf("connector CLI input schema is unsupported: %w", err)
		}
		route.capabilities[commandID] = connectorCommand{capability: connectorCapability(commandID, route.connectorKey, manifestCommand.Name, manifestCommand.Description, manifestCommand.InputSchema),
			invoke: func(callCtx context.Context, request cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
				if !host.routeCurrent(route) {
					return cliservice.CommandOutput{}, cliservice.ErrServiceUnavailable
				}
				timeoutCtx, cancel := context.WithTimeout(callCtx, time.Duration(manifestCommand.TimeoutMS)*time.Millisecond)
				defer cancel()
				arguments := append([]string{}, launchArguments...)
				arguments = append(arguments, managed.CLI.Arguments...)
				arguments = append(arguments, manifestCommand.Arguments...)
				connection, processID, err := host.startRouteProcess(timeoutCtx, route, connectorruntime.ConnectorProcessSpec(route.connectionID, route.connectorKey, managed.Runtime.Language, launchExecutable, prepared.PreparedPath, arguments, sandbox))
				if err != nil {
					return cliservice.CommandOutput{}, cliservice.ServiceUnavailableError("start connector CLI command", err)
				}
				defer route.releaseProcess(processID, connection)
				input, _ := json.Marshal(request.Input)
				if err := connection.Send(append(input, '\n')); err != nil {
					return cliservice.CommandOutput{}, err
				}
				if graceful, ok := connection.(agentruntime.GracefulProcessConnection); ok {
					_ = graceful.CloseInput()
				}
				return collectCLIOutput(timeoutCtx, connection)
			}}
	}
	return nil
}

func (host *ImplementationHost) attachGenericCLI(route *connectorRoute, managed *market.ManagedStdioImplementation,
	prepared market.PreparedArtifactReceipt, launchArguments []string, executable connectorruntime.ConnectorExecutable,
	sandbox *agentruntime.ConnectorSandboxPolicy) error {
	commandID, err := connectorCapabilityID(route.connectorKey, "cli", "run")
	if err != nil {
		return err
	}
	inputSchema := map[string]any{"type": "object", "properties": map[string]any{
		"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
			"description": "CLI arguments described by the installed connector skill"}},
		"required": []string{"args"}, "additionalProperties": false}
	route.capabilities[commandID] = connectorCommand{capability: connectorCapability(commandID, route.connectorKey, "run",
		"Run the installed connector CLI with skill-defined arguments", inputSchema),
		invoke: func(callCtx context.Context, request cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
			if !host.routeCurrent(route) {
				return cliservice.CommandOutput{}, cliservice.ErrServiceUnavailable
			}
			arguments, err := genericCLIArguments(request.Input["args"])
			if err != nil {
				return cliservice.CommandOutput{}, cliservice.InvalidInputReasonError("connector_cli_args_invalid", err.Error(), nil)
			}
			timeoutCtx, cancel := context.WithTimeout(callCtx, time.Duration(managed.CLI.TimeoutMS)*time.Millisecond)
			defer cancel()
			processArguments := append([]string{}, launchArguments...)
			processArguments = append(processArguments, managed.CLI.Arguments...)
			processArguments = append(processArguments, arguments...)
			connection, processID, err := host.startRouteProcess(timeoutCtx, route, connectorruntime.ConnectorProcessSpec(route.connectionID, route.connectorKey,
				managed.Runtime.Language, executable, prepared.PreparedPath, processArguments, sandbox))
			if err != nil {
				return cliservice.CommandOutput{}, cliservice.ServiceUnavailableError("start connector CLI command", err)
			}
			defer route.releaseProcess(processID, connection)
			if graceful, ok := connection.(agentruntime.GracefulProcessConnection); ok {
				_ = graceful.CloseInput()
			}
			return collectCLIOutput(timeoutCtx, connection)
		}}
	return nil
}

func genericCLIArguments(raw any) ([]string, error) {
	var arguments []string
	switch values := raw.(type) {
	case []string:
		arguments = append(arguments, values...)
	case []any:
		for _, value := range values {
			argument, ok := value.(string)
			if !ok {
				return nil, errors.New("connector CLI args must contain only strings")
			}
			arguments = append(arguments, argument)
		}
	default:
		return nil, errors.New("connector CLI args are required")
	}
	for _, argument := range arguments {
		if strings.ContainsRune(argument, '\x00') || argument == "--yes" || argument == "--force" ||
			strings.HasPrefix(argument, "--yes=") || strings.HasPrefix(argument, "--force=") {
			return nil, errors.New("connector CLI args contain a forbidden non-interactive override")
		}
	}
	return arguments, nil
}

func (host *ImplementationHost) startRouteProcess(ctx context.Context, route *connectorRoute, spec agentruntime.ProcessSpec) (agentruntime.ProcessConnection, uint64, error) {
	if !host.routes.IsCurrent(route) {
		return nil, 0, cliservice.ErrServiceUnavailable
	}
	startContext, processID, accepted := route.beginProcess(ctx)
	if !accepted {
		return nil, 0, cliservice.ErrServiceUnavailable
	}
	connection, err := host.awaitProcessStart(ctx, route, processID, startContext, spec)
	if err != nil {
		return nil, 0, err
	}
	return connection, processID, nil
}

func (host *ImplementationHost) awaitProcessStart(waitCtx context.Context, route *connectorRoute, processID uint64,
	startContext context.Context, spec agentruntime.ProcessSpec) (agentruntime.ProcessConnection, error) {
	type startResult struct {
		connection agentruntime.ProcessConnection
		err        error
	}
	result := make(chan startResult, 1)
	go func() {
		connection, err := host.processes.Start(startContext, spec)
		result <- startResult{connection: connection, err: err}
	}()
	select {
	case started := <-result:
		if started.err != nil {
			route.failProcessStart(processID)
			return nil, started.err
		}
		if !route.commitProcessStart(processID, started.connection) {
			_ = started.connection.Close()
			return nil, cliservice.ErrServiceUnavailable
		}
		return started.connection, nil
	case <-waitCtx.Done():
		route.failProcessStart(processID)
		go func() {
			started := <-result
			if started.connection != nil {
				_ = started.connection.Close()
			}
		}()
		return nil, waitCtx.Err()
	}
}

func collectCLIOutput(ctx context.Context, connection agentruntime.ProcessConnection) (cliservice.CommandOutput, error) {
	const maxCLIOutputBytes = 4 << 20
	const maxCLIStderrBytes = 64 << 10
	var stdout, stderr strings.Builder
	for {
		var frame agentruntime.ProcessFrame
		var err error
		if contextual, ok := connection.(agentruntime.ContextProcessConnection); ok {
			frame, err = contextual.RecvContext(ctx)
		} else {
			frame, err = connection.Recv()
		}
		if err != nil {
			if errors.Is(err, io.EOF) && stdout.Len() != 0 {
				break
			}
			return cliservice.CommandOutput{}, cliservice.ServiceUnavailableError("read connector CLI output", err)
		}
		stdout.Write(frame.Stdout)
		stderr.Write(frame.Stderr)
		if stdout.Len() > maxCLIOutputBytes || stderr.Len() > maxCLIStderrBytes {
			if graceful, ok := connection.(agentruntime.GracefulProcessConnection); ok {
				_ = graceful.Kill()
			}
			return cliservice.CommandOutput{}, cliservice.WorkspaceOperationError("connector CLI output exceeded its limit", nil)
		}
		if frame.ExitCode != nil {
			if *frame.ExitCode != 0 {
				return cliservice.CommandOutput{}, cliservice.WorkspaceOperationError(strings.TrimSpace(stderr.String()), nil)
			}
			break
		}
	}
	return jsonCommandOutput([]byte(strings.TrimSpace(stdout.String())))
}

func jsonCommandOutput(raw []byte) (cliservice.CommandOutput, error) {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return cliservice.CommandOutput{}, cliservice.WorkspaceOperationError("connector returned invalid JSON", nil)
	}
	if object, ok := value.(map[string]any); ok {
		return cliservice.CommandOutput{Kind: cliservice.OutputModeJSON, Value: object}, nil
	}
	return cliservice.CommandOutput{Kind: cliservice.OutputModeJSON, Value: map[string]any{"result": value}}, nil
}

func connectorCapability(routeID, connectorKey, name, description string, inputSchema map[string]any) cliservice.Capability {
	if strings.TrimSpace(description) == "" {
		description = "Connector command " + name
	}
	kind := "cli"
	if strings.Contains(routeID, ".mcp.") {
		kind = "mcp"
	}
	return cliservice.Capability{ID: routeID, Path: []string{"connector", connectorKey, kind, name}, Summary: description,
		Description: description, Visibility: cliservice.CapabilityVisibilityPublic, InputSchema: inputSchema,
		Output: cliservice.CapabilityOutput{DefaultMode: cliservice.OutputModeJSON, JSON: true},
		Source: cliservice.CapabilitySource{Kind: cliservice.CapabilitySourceApp, AppID: "connector:" + connectorKey, AppName: connectorKey}}
}

var capabilityPartPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
var hostIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,190}$`)

func connectorCapabilityID(connectorKey, kind, name string) (string, error) {
	if !capabilityPartPattern.MatchString(connectorKey) || !capabilityPartPattern.MatchString(name) {
		return "", errors.New("connector capability name is invalid")
	}
	return "connector." + connectorKey + "." + kind + "." + name, nil
}

func connectorRouteKey(connectionID, connectorKey string) string {
	return connectionID + "\x00" + connectorKey
}

func (host *ImplementationHost) commitRoute(_ string, next *connectorRoute) error {
	return host.routes.Commit(next)
}

func (host *ImplementationHost) removeRoute(key string, generation market.HostGeneration, releaseDigest string, deadline time.Time) error {
	return host.routes.Remove(key, generation, releaseDigest, deadline)
}

func (host *ImplementationHost) retireExactRoute(route *connectorRoute, deadline time.Time) error {
	return host.routes.RetireExact(route, deadline)
}

func (host *ImplementationHost) routeCurrent(route *connectorRoute) bool {
	return host.routes.IsCurrent(route) && !route.processes.IsFenced()
}

func (route *connectorRoute) RouteID() string { return route.id }

func (route *connectorRoute) RouteGeneration() market.HostGeneration { return route.generation }

func (route *connectorRoute) RouteReleaseDigest() string { return route.releaseDigest }

func (route *connectorRoute) Fence() { route.processes.Fence() }

func (route *connectorRoute) Close(deadline time.Time) error {
	if route == nil {
		return nil
	}
	route.closeMu.Lock()
	defer route.closeMu.Unlock()
	closeErr := route.processes.Close(deadline)
	if route.remoteMCP != nil {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		remoteErr := route.remoteMCP.Close(ctx)
		cancel()
		closeErr = errors.Join(closeErr, remoteErr)
		route.remoteMCP = nil
	}
	if closeErr == nil && route.executionRoot != "" {
		if err := route.snapshots.Remove(route.executionRoot); err != nil {
			closeErr = err
		} else {
			route.executionRoot = ""
		}
	}
	return closeErr
}

func (route *connectorRoute) close(deadline time.Time) error { return route.Close(deadline) }

func (route *connectorRoute) beginProcess(parent context.Context) (context.Context, uint64, bool) {
	return route.processes.Begin(parent)
}

func (route *connectorRoute) failProcessStart(processID uint64) {
	route.processes.FailStart(processID)
}

func (route *connectorRoute) commitProcessStart(processID uint64, connection agentruntime.ProcessConnection) bool {
	return route.processes.CommitStart(processID, connection)
}

func (route *connectorRoute) releaseProcess(processID uint64, connection agentruntime.ProcessConnection) {
	route.processes.Release(processID, connection)
}

type ConnectorCommandRegistry struct {
	mu     sync.RWMutex
	routes *connectorruntime.RouteTable
}

type connectorBrokerRoute struct {
	connectorKey  string
	displayName   string
	description   string
	installedRoot string
}

func (registry *ConnectorCommandRegistry) routesFor() []connectorBrokerRoute {
	routes := registry.activeRoutes()
	result := make([]connectorBrokerRoute, 0, len(routes))
	for _, route := range routes {
		result = append(result, connectorBrokerRoute{connectorKey: route.connectorKey, displayName: route.displayName,
			description: route.description, installedRoot: route.installedRoot})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].connectorKey < result[right].connectorKey })
	return result
}

func (registry *ConnectorCommandRegistry) invokeInternal(ctx context.Context, connectorKey, capabilityName string, request cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
	routes := registry.activeRoutes()
	if len(routes) == 0 {
		return cliservice.CommandOutput{}, cliservice.ErrServiceUnavailable
	}
	matches := make([]connectorCommand, 0, 1)
	for _, route := range routes {
		if route.connectorKey != connectorKey {
			continue
		}
		for _, command := range route.capabilities {
			parts := strings.Split(command.capability.ID, ".")
			if len(parts) >= 4 && strings.Join(parts[3:], ".") == capabilityName {
				matches = append(matches, command)
			}
		}
	}
	if len(matches) == 0 {
		return cliservice.CommandOutput{}, cliservice.InvalidInputReasonError("connector_capability_not_found", "Connector capability was not found", nil)
	}
	if len(matches) > 1 {
		return cliservice.CommandOutput{}, cliservice.InvalidInputReasonError("connector_capability_ambiguous", "Connector capability name is ambiguous", nil)
	}
	return matches[0].invoke(ctx, request)
}

func NewConnectorCommandRegistry() *ConnectorCommandRegistry {
	return &ConnectorCommandRegistry{}
}

func (registry *ConnectorCommandRegistry) attach(routes *connectorruntime.RouteTable) {
	registry.mu.Lock()
	registry.routes = routes
	registry.mu.Unlock()
}

func (registry *ConnectorCommandRegistry) activeRoutes() []*connectorRoute {
	registry.mu.RLock()
	table := registry.routes
	registry.mu.RUnlock()
	if table == nil {
		return nil
	}
	portable := table.PublishedRoutes()
	routes := make([]*connectorRoute, 0, len(portable))
	for _, candidate := range portable {
		if route, ok := candidate.(*connectorRoute); ok {
			routes = append(routes, route)
		}
	}
	return routes
}

func (registry *ConnectorCommandRegistry) Capabilities(_ context.Context, _ cliservice.InvokeContext) []cliservice.Capability {
	result := []cliservice.Capability{}
	for _, route := range registry.activeRoutes() {
		for _, command := range route.capabilities {
			result = append(result, command.capability)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func (registry *ConnectorCommandRegistry) Invoke(ctx context.Context, request cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
	routes := registry.activeRoutes()
	if len(routes) == 0 {
		return cliservice.CommandOutput{}, cliservice.ErrServiceUnavailable
	}
	var command *connectorCommand
	for _, route := range routes {
		if candidate, ok := route.capabilities[request.CommandID]; ok {
			copy := candidate
			command = &copy
			break
		}
	}
	if command == nil {
		return cliservice.CommandOutput{}, cliservice.ErrCommandNotFound
	}
	return command.invoke(ctx, request)
}

func ProductionPorts(host *ImplementationHost, authorization market.AuthorizationProvider) (market.ImplementationHost, market.AuthorizationProvider, market.CompatibilityEvaluator, market.ImplementationRegistry) {
	if authorization == nil {
		authorization = unavailableAuthorization{}
	}
	return host, authorization, productionCompatibility{}, market.NewImplementationRegistry(map[string]market.ImplementationValidator{
		market.ImplementationKindManagedStdio:         nil,
		market.ImplementationKindRemoteStreamableHTTP: nil,
	})
}

// unavailableAuthorization is a Tutti product adapter: the current production
// runtime intentionally admits only connectors whose authorization kind is
// "none". Shared Host lifecycle code must not infer that product capability.
type unavailableAuthorization struct{}

func (unavailableAuthorization) Begin(context.Context, market.AuthorizationStartRequest) (market.AuthorizationSession, error) {
	return market.AuthorizationSession{}, errors.New("connector authorization is not registered")
}

func (unavailableAuthorization) Disconnect(context.Context, market.AuthorizationDisconnectRequest) error {
	return errors.New("connector authorization is not registered")
}

type productionCompatibility struct{}

func (productionCompatibility) Evaluate(manifest market.Manifest) market.Compatibility {
	if manifest.Implementation.Kind == market.ImplementationKindRemoteStreamableHTTP {
		return market.Compatibility{State: market.CompatibilityStateSupported}
	}
	if manifest.Implementation.Kind != market.ImplementationKindManagedStdio || manifest.AuthorizationKind != "none" {
		return market.Compatibility{State: market.CompatibilityStateUnsupportedImplementation, Reason: "implementation or authorization broker is unavailable"}
	}
	for _, platform := range manifest.Compatibility.Platforms {
		if platform == runtime.GOOS+"-"+runtime.GOARCH {
			return market.Compatibility{State: market.CompatibilityStateSupported}
		}
	}
	if len(manifest.Compatibility.Platforms) != 0 {
		return market.Compatibility{State: market.CompatibilityStateUnsupportedPlatform, Reason: "platform is not supported"}
	}
	return market.Compatibility{State: market.CompatibilityStateSupported}
}
