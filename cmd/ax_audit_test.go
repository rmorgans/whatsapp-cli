package cmd

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// expectedCLICommands is the authoritative list of leaf command paths
// that must exist in the Cobra command tree. When a command is added
// or removed, this list must be updated.
var expectedCLICommands = []string{
	"version",
	"auth",
	"sync",
	"send",
	"messages list",
	"messages search",
	"messages react",
	"messages delete",
	"messages edit",
	"messages mark-read",
	"contacts search",
	"contacts block",
	"contacts unblock",
	"contacts list-blocked",
	"contacts check",
	"chats list",
	"media download",
	"groups list",
	"groups info",
	"groups create",
	"groups invite-link",
	"groups join",
	"groups leave",
	"groups add-members",
	"groups remove-members",
	"groups set-name",
	"groups set-description",
	"groups set-photo",
}

// collectLeafPaths walks the Cobra command tree and returns all leaf
// command paths (commands without subcommands).
func collectLeafPaths(cmd *cobra.Command, prefix string) []string {
	var paths []string
	for _, child := range cmd.Commands() {
		path := child.Name()
		if prefix != "" {
			path = prefix + " " + child.Name()
		}
		if child.HasSubCommands() {
			paths = append(paths, collectLeafPaths(child, path)...)
		} else {
			paths = append(paths, path)
		}
	}
	return paths
}

// TestAXCoverage_CobraCommandTreeMatchesExpected verifies the actual
// registered Cobra command tree against the expected command list.
// This catches:
//   - Commands removed from registerAll but still in the expected list
//   - Commands added to registerAll but missing from the expected list
func TestAXCoverage_CobraCommandTreeMatchesExpected(t *testing.T) {
	// Build the full command tree via the normal init path.
	SetVersion("test")
	root := reg.Root()

	actual := collectLeafPaths(root, "")
	sort.Strings(actual)

	expected := make([]string, len(expectedCLICommands))
	copy(expected, expectedCLICommands)
	sort.Strings(expected)

	// Filter out cobra built-ins (help, completion).
	builtins := map[string]bool{"help": true, "completion": true}
	var filtered []string
	for _, p := range actual {
		if !builtins[p] {
			filtered = append(filtered, p)
		}
	}

	// Check for missing commands (expected but not in tree).
	actualSet := make(map[string]bool, len(filtered))
	for _, p := range filtered {
		actualSet[p] = true
	}
	var missing []string
	for _, p := range expected {
		if !actualSet[p] {
			missing = append(missing, p)
		}
	}
	assert.Empty(t, missing,
		"Commands in expectedCLICommands but NOT in Cobra tree. "+
			"Either register the command or remove it from expectedCLICommands in ax_audit_test.go")

	// Check for unexpected commands (in tree but not expected).
	expectedSet := make(map[string]bool, len(expected))
	for _, p := range expected {
		expectedSet[p] = true
	}
	var unexpected []string
	for _, p := range filtered {
		if !expectedSet[p] {
			unexpected = append(unexpected, p)
		}
	}
	assert.Empty(t, unexpected,
		"Commands in Cobra tree but NOT in expectedCLICommands. "+
			"Add new commands to expectedCLICommands in ax_audit_test.go")
}
