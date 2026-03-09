package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vicentereig/whatsapp-cli/internal/types"
)

func TestGroupsList_Success(t *testing.T) {
	mockClient := &MockWAClient{
		GetJoinedGroupsFunc: func(ctx context.Context) ([]types.GroupInfo, error) {
			return []types.GroupInfo{
				{JID: "group1@g.us", Name: "Family", MemberCount: 5},
				{JID: "group2@g.us", Name: "Work", MemberCount: 12},
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsList(context.Background())

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var groups []types.GroupInfo
	err := json.Unmarshal(resp.Data, &groups)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "Family", groups[0].Name)
	require.Equal(t, "group1@g.us", groups[0].JID)
	require.Equal(t, 5, groups[0].MemberCount)
	require.Equal(t, "Work", groups[1].Name)
}

func TestGroupsList_Error(t *testing.T) {
	mockClient := &MockWAClient{
		GetJoinedGroupsFunc: func(ctx context.Context) ([]types.GroupInfo, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsList(context.Background())

	resp := parseResponse(t, result)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	require.Contains(t, *resp.Error, "network error")
}

func TestGroupsInfo_Success(t *testing.T) {
	mockClient := &MockWAClient{
		GetGroupInfoFunc: func(ctx context.Context, jid string) (*types.GroupInfo, error) {
			require.Equal(t, "group1@g.us", jid)
			return &types.GroupInfo{
				JID:         "group1@g.us",
				Name:        "Family",
				Description: "Family group chat",
				MemberCount: 3,
				Members: []types.GroupParticipant{
					{JID: "alice@s.whatsapp.net", IsAdmin: true},
					{JID: "bob@s.whatsapp.net"},
					{JID: "charlie@s.whatsapp.net"},
				},
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsInfo(context.Background(), "group1@g.us")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var info types.GroupInfo
	err := json.Unmarshal(resp.Data, &info)
	require.NoError(t, err)
	require.Equal(t, "Family", info.Name)
	require.Equal(t, "Family group chat", info.Description)
	require.Len(t, info.Members, 3)
	require.True(t, info.Members[0].IsAdmin)
}

func TestGroupsCreate_Success(t *testing.T) {
	var capturedName string
	var capturedMembers []string

	mockClient := &MockWAClient{
		CreateGroupFunc: func(ctx context.Context, name string, members []string) (*types.GroupInfo, error) {
			capturedName = name
			capturedMembers = members
			return &types.GroupInfo{
				JID:         "newgroup@g.us",
				Name:        name,
				MemberCount: len(members) + 1,
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsCreate(context.Background(), "Test Group", []string{"5511999999999", "5511888888888"})

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var info types.GroupInfo
	err := json.Unmarshal(resp.Data, &info)
	require.NoError(t, err)
	require.Equal(t, "newgroup@g.us", info.JID)
	require.Equal(t, "Test Group", info.Name)

	require.Equal(t, "Test Group", capturedName)
	require.Equal(t, []string{"5511999999999", "5511888888888"}, capturedMembers)
}

func TestGroupsInviteLink_Success(t *testing.T) {
	mockClient := &MockWAClient{
		GetGroupInviteLinkFunc: func(ctx context.Context, jid string, reset bool) (string, error) {
			require.Equal(t, "group1@g.us", jid)
			require.False(t, reset)
			return "https://chat.whatsapp.com/ABC123", nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsInviteLink(context.Background(), "group1@g.us", false)

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, "https://chat.whatsapp.com/ABC123", data["link"])
	require.Equal(t, "group1@g.us", data["jid"])
}

func TestGroupsJoin_Success(t *testing.T) {
	mockClient := &MockWAClient{
		JoinGroupWithLinkFunc: func(ctx context.Context, link string) (string, error) {
			require.Equal(t, "https://chat.whatsapp.com/ABC123", link)
			return "newgroup@g.us", nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsJoin(context.Background(), "https://chat.whatsapp.com/ABC123")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["joined"])
	require.Equal(t, "newgroup@g.us", data["jid"])
}

func TestGroupsLeave_Success(t *testing.T) {
	var capturedJID string
	mockClient := &MockWAClient{
		LeaveGroupFunc: func(ctx context.Context, jid string) error {
			capturedJID = jid
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsLeave(context.Background(), "group1@g.us")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["left"])
	require.Equal(t, "group1@g.us", data["jid"])
	require.Equal(t, "group1@g.us", capturedJID)
}

func TestGroupsAddMembers_Success(t *testing.T) {
	var capturedJID, capturedAction string
	var capturedMembers []string

	mockClient := &MockWAClient{
		UpdateGroupParticipantsFunc: func(ctx context.Context, jid string, members []string, action string) error {
			capturedJID = jid
			capturedMembers = members
			capturedAction = action
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsAddMembers(context.Background(), "group1@g.us", []string{"5511999999999"})

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["added"])

	require.Equal(t, "group1@g.us", capturedJID)
	require.Equal(t, []string{"5511999999999"}, capturedMembers)
	require.Equal(t, "add", capturedAction)
}

func TestGroupsRemoveMembers_Success(t *testing.T) {
	var capturedAction string

	mockClient := &MockWAClient{
		UpdateGroupParticipantsFunc: func(ctx context.Context, jid string, members []string, action string) error {
			capturedAction = action
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsRemoveMembers(context.Background(), "group1@g.us", []string{"5511999999999"})

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	require.Equal(t, "remove", capturedAction)
}

func TestGroupsSetName_Success(t *testing.T) {
	var capturedJID, capturedName string

	mockClient := &MockWAClient{
		SetGroupNameFunc: func(ctx context.Context, jid, name string) error {
			capturedJID = jid
			capturedName = name
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsSetName(context.Background(), "group1@g.us", "New Name")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["updated"])
	require.Equal(t, "New Name", data["name"])

	require.Equal(t, "group1@g.us", capturedJID)
	require.Equal(t, "New Name", capturedName)
}

func TestGroupsSetDescription_Success(t *testing.T) {
	var capturedJID, capturedDesc string

	mockClient := &MockWAClient{
		SetGroupDescriptionFunc: func(ctx context.Context, jid, description string) error {
			capturedJID = jid
			capturedDesc = description
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsSetDescription(context.Background(), "group1@g.us", "New description")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["updated"])
	require.Equal(t, "New description", data["description"])

	require.Equal(t, "group1@g.us", capturedJID)
	require.Equal(t, "New description", capturedDesc)
}

func TestGroupsSetPhoto_Success(t *testing.T) {
	var capturedJID, capturedPath string

	mockClient := &MockWAClient{
		SetGroupPhotoFunc: func(ctx context.Context, jid, imagePath string) error {
			capturedJID = jid
			capturedPath = imagePath
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsSetPhoto(context.Background(), "group1@g.us", "/tmp/photo.jpg")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["updated"])
	require.Equal(t, "group1@g.us", data["jid"])

	require.Equal(t, "group1@g.us", capturedJID)
	require.Equal(t, "/tmp/photo.jpg", capturedPath)
}

func TestGroupsSetPhoto_Error(t *testing.T) {
	mockClient := &MockWAClient{
		SetGroupPhotoFunc: func(ctx context.Context, jid, imagePath string) error {
			return fmt.Errorf("invalid image format")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result := app.GroupsSetPhoto(context.Background(), "group1@g.us", "/tmp/bad.png")

	resp := parseResponse(t, result)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	require.Contains(t, *resp.Error, "invalid image format")
}

func TestGroupsConnectError(t *testing.T) {
	mockClient := &MockWAClient{
		ConnectFunc: func(ctx context.Context) error {
			return fmt.Errorf("connection refused")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	// Every group command should fail with a connect error.
	tests := []struct {
		name string
		fn   func() string
	}{
		{"list", func() string { return app.GroupsList(context.Background()) }},
		{"info", func() string { return app.GroupsInfo(context.Background(), "g@g.us") }},
		{"create", func() string { return app.GroupsCreate(context.Background(), "n", nil) }},
		{"invite-link", func() string { return app.GroupsInviteLink(context.Background(), "g@g.us", false) }},
		{"join", func() string { return app.GroupsJoin(context.Background(), "https://link") }},
		{"leave", func() string { return app.GroupsLeave(context.Background(), "g@g.us") }},
		{"add-members", func() string { return app.GroupsAddMembers(context.Background(), "g@g.us", []string{"m"}) }},
		{"remove-members", func() string { return app.GroupsRemoveMembers(context.Background(), "g@g.us", []string{"m"}) }},
		{"set-name", func() string { return app.GroupsSetName(context.Background(), "g@g.us", "n") }},
		{"set-description", func() string { return app.GroupsSetDescription(context.Background(), "g@g.us", "d") }},
		{"set-photo", func() string { return app.GroupsSetPhoto(context.Background(), "g@g.us", "/img") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := parseResponse(t, tt.fn())
			require.False(t, resp.Success)
			require.NotNil(t, resp.Error)
			require.Contains(t, *resp.Error, "connection refused")
		})
	}
}
