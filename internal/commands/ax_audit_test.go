package commands

import (
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// WAClient methods that are infrastructure, not user-facing commands.
// These are intentionally not exposed as CLI commands.
var waClientInfrastructureMethods = map[string]bool{
	"Connect":         true, // Called internally before each command
	"Disconnect":      true, // Called internally on close
	"IsAuthenticated": true, // Called internally by Auth
	"GetOwnJID":       true, // Called internally by Sync
	"ResolveChatName": true, // Called internally by Sync
	"StartSync":       true, // Called internally by Sync command
}

// Mapping from WAClient method → CLI command that exposes it.
// This is the authoritative mapping. When a new method is added to
// WAClient, this test will fail until the method is either mapped
// here or added to waClientInfrastructureMethods.
var waClientMethodToCLI = map[string]string{
	"Authenticate":            "auth",
	"SendMessage":             "send --message",
	"SendImageMessage":        "send --image",
	"SendVideoMessage":        "send --video",
	"SendAudioMessage":        "send --audio",
	"SendDocumentMessage":     "send --document",
	"SendTextReply":           "send --message --reply-to",
	"DownloadMediaToFile":     "media download",
	"ReactToMessage":          "messages react",
	"RevokeMessage":           "messages delete",
	"EditMessage":             "messages edit",
	"MarkRead":                "messages mark-read",
	"UpdateBlocklist":         "contacts block / contacts unblock",
	"GetBlocklist":            "contacts list-blocked",
	"IsOnWhatsApp":            "contacts check",
	"GetJoinedGroups":         "groups list",
	"GetGroupInfo":            "groups info",
	"CreateGroup":             "groups create",
	"GetGroupInviteLink":      "groups invite-link",
	"JoinGroupWithLink":       "groups join",
	"LeaveGroup":              "groups leave",
	"UpdateGroupParticipants": "groups add-members / groups remove-members",
	"SetGroupName":            "groups set-name",
	"SetGroupDescription":     "groups set-description",
	"SetGroupPhoto":           "groups set-photo",
}

// TestAXCoverage_WAClientMethodsMapped ensures every method on the WAClient
// interface is either mapped in the tracking table or explicitly marked as
// infrastructure. This catches new whatsmeow capabilities at the interface
// level. The actual Cobra command tree is verified separately in cmd/ax_audit_test.go.
func TestAXCoverage_WAClientMethodsMapped(t *testing.T) {
	typ := reflect.TypeOf((*WAClient)(nil)).Elem()

	var unmapped []string
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if waClientInfrastructureMethods[name] {
			continue
		}
		if _, ok := waClientMethodToCLI[name]; !ok {
			unmapped = append(unmapped, name)
		}
	}

	sort.Strings(unmapped)
	assert.Empty(t, unmapped,
		"WAClient methods not mapped to CLI commands or marked as infrastructure. "+
			"Add them to waClientMethodToCLI or waClientInfrastructureMethods in ax_audit_test.go")
}

// TestAXCoverage_AppPublicMethodsExist ensures every CLI-mapped WAClient
// method has a corresponding public method on App. This catches cases where
// the mapping table references a method that was removed or renamed.
func TestAXCoverage_AppPublicMethodsExist(t *testing.T) {
	appType := reflect.TypeOf(&App{})

	// App methods that correspond to WAClient methods.
	// These are the public App methods called by cmd/commands.go.
	expectedAppMethods := []string{
		"Auth",
		"Sync",
		"SendMessage",
		"SendImage",
		"SendVideo",
		"SendAudio",
		"SendDocument",
		"SendReply",
		"DownloadMedia",
		"ReactToMessage",
		"DeleteMessage",
		"EditMessage",
		"MarkMessageRead",
		"ListMessages",
		"SearchContacts",
		"ListChats",
		"BlockContact",
		"UnblockContact",
		"ListBlocked",
		"CheckOnWhatsApp",
		"GroupsList",
		"GroupsInfo",
		"GroupsCreate",
		"GroupsInviteLink",
		"GroupsJoin",
		"GroupsLeave",
		"GroupsAddMembers",
		"GroupsRemoveMembers",
		"GroupsSetName",
		"GroupsSetDescription",
		"GroupsSetPhoto",
	}

	var missing []string
	for _, name := range expectedAppMethods {
		if _, ok := appType.MethodByName(name); !ok {
			missing = append(missing, name)
		}
	}

	assert.Empty(t, missing,
		"App methods expected by CLI commands not found. "+
			"Update the App type or expectedAppMethods in ax_audit_test.go")
}

// TestAXCoverage_MappingTableComplete verifies the mapping table accounts
// for all non-infrastructure WAClient methods (no stale entries).
func TestAXCoverage_MappingTableComplete(t *testing.T) {
	typ := reflect.TypeOf((*WAClient)(nil)).Elem()

	interfaceMethods := make(map[string]bool)
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if !waClientInfrastructureMethods[name] {
			interfaceMethods[name] = true
		}
	}

	// Check for stale mapping entries.
	var stale []string
	for name := range waClientMethodToCLI {
		if !interfaceMethods[name] {
			stale = append(stale, name)
		}
	}

	sort.Strings(stale)
	assert.Empty(t, stale,
		"waClientMethodToCLI references WAClient methods that no longer exist. "+
			"Remove stale entries from ax_audit_test.go")
}
