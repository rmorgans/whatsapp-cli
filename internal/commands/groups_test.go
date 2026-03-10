package commands

import (
	"context"
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

	groups, err := app.GroupsList(context.Background())
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

	result, err := app.GroupsList(context.Background())
	require.ErrorContains(t, err, "network error")
	require.Nil(t, result)
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

	info, err := app.GroupsInfo(context.Background(), "group1@g.us")
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

	info, err := app.GroupsCreate(context.Background(), "Test Group", []string{"5511999999999", "5511888888888"})
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

	result, err := app.GroupsInviteLink(context.Background(), "group1@g.us", false)
	require.NoError(t, err)
	require.Equal(t, "https://chat.whatsapp.com/ABC123", result.Link)
	require.Equal(t, "group1@g.us", result.JID)
}

func TestGroupsJoin_Success(t *testing.T) {
	mockClient := &MockWAClient{
		JoinGroupWithLinkFunc: func(ctx context.Context, link string) (string, error) {
			require.Equal(t, "https://chat.whatsapp.com/ABC123", link)
			return "newgroup@g.us", nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.GroupsJoin(context.Background(), "https://chat.whatsapp.com/ABC123")
	require.NoError(t, err)
	require.True(t, result.Joined)
	require.Equal(t, "newgroup@g.us", result.JID)
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

	result, err := app.GroupsLeave(context.Background(), "group1@g.us")
	require.NoError(t, err)
	require.True(t, result.Left)
	require.Equal(t, "group1@g.us", result.JID)
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

	result, err := app.GroupsAddMembers(context.Background(), "group1@g.us", []string{"5511999999999"})
	require.NoError(t, err)
	require.True(t, result.Added)

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

	result, err := app.GroupsRemoveMembers(context.Background(), "group1@g.us", []string{"5511999999999"})
	require.NoError(t, err)
	require.True(t, result.Removed)
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

	result, err := app.GroupsSetName(context.Background(), "group1@g.us", "New Name")
	require.NoError(t, err)
	require.True(t, result.Updated)
	require.Equal(t, "New Name", result.Name)

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

	result, err := app.GroupsSetDescription(context.Background(), "group1@g.us", "New description")
	require.NoError(t, err)
	require.True(t, result.Updated)
	require.Equal(t, "New description", result.Description)

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

	result, err := app.GroupsSetPhoto(context.Background(), "group1@g.us", "/tmp/photo.jpg")
	require.NoError(t, err)
	require.True(t, result.Updated)
	require.Equal(t, "group1@g.us", result.JID)

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

	_, err := app.GroupsSetPhoto(context.Background(), "group1@g.us", "/tmp/bad.png")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid image format")
}

func TestGroupsConnectError(t *testing.T) {
	mockClient := &MockWAClient{
		ConnectFunc: func(ctx context.Context) error {
			return fmt.Errorf("connection refused")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	t.Run("list", func(t *testing.T) {
		result, err := app.GroupsList(context.Background())
		require.ErrorContains(t, err, "connection refused")
		require.Nil(t, result)
	})
	t.Run("info", func(t *testing.T) {
		result, err := app.GroupsInfo(context.Background(), "g@g.us")
		require.ErrorContains(t, err, "connection refused")
		require.Nil(t, result)
	})
	t.Run("create", func(t *testing.T) {
		result, err := app.GroupsCreate(context.Background(), "n", nil)
		require.ErrorContains(t, err, "connection refused")
		require.Nil(t, result)
	})
	t.Run("invite-link", func(t *testing.T) {
		_, err := app.GroupsInviteLink(context.Background(), "g@g.us", false)
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("join", func(t *testing.T) {
		_, err := app.GroupsJoin(context.Background(), "https://link")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("leave", func(t *testing.T) {
		_, err := app.GroupsLeave(context.Background(), "g@g.us")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("add-members", func(t *testing.T) {
		_, err := app.GroupsAddMembers(context.Background(), "g@g.us", []string{"m"})
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("remove-members", func(t *testing.T) {
		_, err := app.GroupsRemoveMembers(context.Background(), "g@g.us", []string{"m"})
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("set-name", func(t *testing.T) {
		_, err := app.GroupsSetName(context.Background(), "g@g.us", "n")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("set-description", func(t *testing.T) {
		_, err := app.GroupsSetDescription(context.Background(), "g@g.us", "d")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("set-photo", func(t *testing.T) {
		_, err := app.GroupsSetPhoto(context.Background(), "g@g.us", "/img")
		require.ErrorContains(t, err, "connection refused")
	})
}
