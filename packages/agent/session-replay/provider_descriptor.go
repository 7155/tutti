package sessionreplay

import "strings"

type ProviderTapeCodec string

const ProviderTapeCodecJSONRPC ProviderTapeCodec = "json-rpc"

type ProviderRequestMatcher string

const ProviderRequestMatcherJSONRPC ProviderRequestMatcher = "json-rpc"

type ProviderInputObserver string

const ProviderInputObserverACPJSON ProviderInputObserver = "acp-json-input-units"

type ProviderProjectionCodec string

const ProviderProjectionCodecJSONRPCPortable ProviderProjectionCodec = "json-rpc-portable-v1"

type ProviderAuditCodec string

const ProviderAuditCodecJSONRPCPortable ProviderAuditCodec = "json-rpc-portable-v1"

type ProviderGeneratedRequestField struct {
	Method        string
	Parameter     string
	ValuePrefix   string
	PortableValue string
}

type ProviderTapeDescriptor struct {
	Codec                  ProviderTapeCodec
	RequestMatcher         ProviderRequestMatcher
	InputObserver          ProviderInputObserver
	ProjectionCodec        ProviderProjectionCodec
	AuditCodec             ProviderAuditCodec
	SchemaVersion          int
	ProjectionVersion      int
	CredentialMethods      []string
	OptionalProbeMethods   []string
	AccountReadMethod      string
	GeneratedRequestFields []ProviderGeneratedRequestField
}

type ProviderPortableRuntimeDescriptor struct {
	HomeEnvVars          []string
	SessionHomeDirectory string
}

// ProviderReplayDescriptor is the registration point for one Provider's
// recording and replay behavior. Consumers dispatch on the declared codec and
// matcher strategies rather than on Provider identity.
type ProviderReplayDescriptor struct {
	ProviderID      string
	AgentTargetID   string
	Tape            ProviderTapeDescriptor
	PortableRuntime ProviderPortableRuntimeDescriptor
}

var providerReplayDescriptors = []ProviderReplayDescriptor{
	{
		ProviderID:    "codex",
		AgentTargetID: "local:codex",
		Tape: ProviderTapeDescriptor{
			Codec:                ProviderTapeCodecJSONRPC,
			RequestMatcher:       ProviderRequestMatcherJSONRPC,
			InputObserver:        ProviderInputObserverACPJSON,
			ProjectionCodec:      ProviderProjectionCodecJSONRPCPortable,
			AuditCodec:           ProviderAuditCodecJSONRPCPortable,
			SchemaVersion:        ProcessCassetteSchemaVersion,
			ProjectionVersion:    ProcessCassetteProjectionVersion,
			CredentialMethods:    []string{"account/login/start", "account/chatgptAuthTokens/refresh"},
			OptionalProbeMethods: []string{"thread/read", "thread/goal/get"},
			AccountReadMethod:    "account/read",
			GeneratedRequestFields: []ProviderGeneratedRequestField{{
				Method:        "turn/start",
				Parameter:     "clientUserMessageId",
				ValuePrefix:   "plan-decision:",
				PortableValue: "plan-decision:<runtime-operation>",
			}},
		},
		PortableRuntime: ProviderPortableRuntimeDescriptor{
			HomeEnvVars:          []string{"CODEX_HOME"},
			SessionHomeDirectory: "codex-home",
		},
	},
}

func FindProviderReplayByTarget(agentTargetID string) (ProviderReplayDescriptor, bool) {
	want := normalizeProviderReplayIdentity(agentTargetID)
	for _, descriptor := range providerReplayDescriptors {
		if normalizeProviderReplayIdentity(descriptor.AgentTargetID) == want {
			return cloneProviderReplayDescriptor(descriptor), true
		}
	}
	return ProviderReplayDescriptor{}, false
}

func FindProviderReplayByProvider(providerID string) (ProviderReplayDescriptor, bool) {
	want := normalizeProviderReplayIdentity(providerID)
	for _, descriptor := range providerReplayDescriptors {
		if normalizeProviderReplayIdentity(descriptor.ProviderID) == want {
			return cloneProviderReplayDescriptor(descriptor), true
		}
	}
	return ProviderReplayDescriptor{}, false
}

// ResolveProviderReplay resolves one canonical Session binding. When both
// identities are present they must name the same registered adapter.
func ResolveProviderReplay(
	agentTargetID string,
	providerID string,
) (ProviderReplayDescriptor, bool) {
	targetID := normalizeProviderReplayIdentity(agentTargetID)
	providerID = normalizeProviderReplayIdentity(providerID)
	if targetID == "" {
		return FindProviderReplayByProvider(providerID)
	}
	if providerID == "" {
		return FindProviderReplayByTarget(targetID)
	}
	byTarget, targetFound := FindProviderReplayByTarget(targetID)
	byProvider, providerFound := FindProviderReplayByProvider(providerID)
	if !targetFound || !providerFound ||
		normalizeProviderReplayIdentity(byTarget.ProviderID) !=
			normalizeProviderReplayIdentity(byProvider.ProviderID) {
		return ProviderReplayDescriptor{}, false
	}
	return byTarget, true
}

func (d ProviderReplayDescriptor) SupportsFormat(schemaVersion, projectionVersion int) bool {
	return d.Tape.SchemaVersion == schemaVersion &&
		d.Tape.ProjectionVersion == projectionVersion
}

func (d ProviderReplayDescriptor) MethodCarriesCredentials(method string) bool {
	return containsProviderReplayValue(d.Tape.CredentialMethods, method)
}

func (d ProviderReplayDescriptor) IsOptionalProbeMethod(method string) bool {
	return containsProviderReplayValue(d.Tape.OptionalProbeMethods, method)
}

func (d ProviderReplayDescriptor) IsHomeEnvVar(name string) bool {
	return containsProviderReplayValue(d.PortableRuntime.HomeEnvVars, name)
}

func containsProviderReplayValue(values []string, value string) bool {
	want := normalizeProviderReplayIdentity(value)
	for _, candidate := range values {
		if normalizeProviderReplayIdentity(candidate) == want {
			return true
		}
	}
	return false
}

func normalizeProviderReplayIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneProviderReplayDescriptor(source ProviderReplayDescriptor) ProviderReplayDescriptor {
	cloned := source
	cloned.Tape.CredentialMethods = append([]string(nil), source.Tape.CredentialMethods...)
	cloned.Tape.OptionalProbeMethods = append([]string(nil), source.Tape.OptionalProbeMethods...)
	cloned.Tape.GeneratedRequestFields = append(
		[]ProviderGeneratedRequestField(nil),
		source.Tape.GeneratedRequestFields...,
	)
	cloned.PortableRuntime.HomeEnvVars = append(
		[]string(nil),
		source.PortableRuntime.HomeEnvVars...,
	)
	return cloned
}
