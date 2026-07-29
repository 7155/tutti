package agentruntime

import (
	"fmt"
	"os"

	replay "github.com/tutti-os/tutti/packages/agent/session-replay"
)

// AuditProjectedProcessCassetteFrames verifies that a persisted Provider tape
// contains only the structural values allowed after recording projection.
func AuditProjectedProcessCassetteFrames(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open projected process cassette frames: %w", err)
	}
	defer file.Close()
	return replay.AuditProjectedProcessCassetteFrames(file)
}
