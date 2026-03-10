package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vicentereig/whatsapp-cli/internal/types"
)

func TestBlockContact_Success(t *testing.T) {
	var capturedJID, capturedAction string

	mockClient := &MockWAClient{
		UpdateBlocklistFunc: func(ctx context.Context, jid string, action string) error {
			capturedJID = jid
			capturedAction = action
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.BlockContact(context.Background(), "5511999999999@s.whatsapp.net")
	require.NoError(t, err)
	require.True(t, result.Blocked)
	require.Equal(t, "5511999999999@s.whatsapp.net", result.JID)

	require.Equal(t, "5511999999999@s.whatsapp.net", capturedJID)
	require.Equal(t, "block", capturedAction)
}

func TestBlockContact_Error(t *testing.T) {
	mockClient := &MockWAClient{
		UpdateBlocklistFunc: func(ctx context.Context, jid string, action string) error {
			return fmt.Errorf("server error")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	_, err := app.BlockContact(context.Background(), "bad@s.whatsapp.net")
	require.Error(t, err)
	require.Contains(t, err.Error(), "server error")
}

func TestUnblockContact_Success(t *testing.T) {
	var capturedJID, capturedAction string

	mockClient := &MockWAClient{
		UpdateBlocklistFunc: func(ctx context.Context, jid string, action string) error {
			capturedJID = jid
			capturedAction = action
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.UnblockContact(context.Background(), "5511999999999@s.whatsapp.net")
	require.NoError(t, err)
	require.True(t, result.Unblocked)
	require.Equal(t, "5511999999999@s.whatsapp.net", result.JID)

	require.Equal(t, "5511999999999@s.whatsapp.net", capturedJID)
	require.Equal(t, "unblock", capturedAction)
}

func TestListBlocked_Success(t *testing.T) {
	mockClient := &MockWAClient{
		GetBlocklistFunc: func(ctx context.Context) ([]string, error) {
			return []string{
				"5511999999999@s.whatsapp.net",
				"5511888888888@s.whatsapp.net",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	jids, err := app.ListBlocked(context.Background())
	require.NoError(t, err)
	require.Len(t, jids, 2)
	require.Equal(t, "5511999999999@s.whatsapp.net", jids[0])
	require.Equal(t, "5511888888888@s.whatsapp.net", jids[1])
}

func TestListBlocked_Empty(t *testing.T) {
	mockClient := &MockWAClient{
		GetBlocklistFunc: func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	jids, err := app.ListBlocked(context.Background())
	require.NoError(t, err)
	require.Empty(t, jids)
}

func TestListBlocked_Error(t *testing.T) {
	mockClient := &MockWAClient{
		GetBlocklistFunc: func(ctx context.Context) ([]string, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.ListBlocked(context.Background())
	require.ErrorContains(t, err, "network error")
	require.Nil(t, result)
}

func TestCheckOnWhatsApp_Success(t *testing.T) {
	var capturedPhones []string

	mockClient := &MockWAClient{
		IsOnWhatsAppFunc: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
			capturedPhones = phones
			return []types.IsOnWhatsAppResponse{
				{Query: "+5511999999999", JID: "5511999999999@s.whatsapp.net", IsOnWhatsApp: true},
				{Query: "+5511000000000", JID: "", IsOnWhatsApp: false},
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	results, err := app.CheckOnWhatsApp(context.Background(), []string{"+5511999999999", "+5511000000000"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "+5511999999999", results[0].Query)
	require.Equal(t, "5511999999999@s.whatsapp.net", results[0].JID)
	require.True(t, results[0].IsOnWhatsApp)
	require.Equal(t, "+5511000000000", results[1].Query)
	require.False(t, results[1].IsOnWhatsApp)

	require.Equal(t, []string{"+5511999999999", "+5511000000000"}, capturedPhones)
}

func TestCheckOnWhatsApp_Error(t *testing.T) {
	mockClient := &MockWAClient{
		IsOnWhatsAppFunc: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
			return nil, fmt.Errorf("rate limited")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.CheckOnWhatsApp(context.Background(), []string{"+5511999999999"})
	require.ErrorContains(t, err, "rate limited")
	require.Nil(t, result)
}

func TestContactsConnectError(t *testing.T) {
	mockClient := &MockWAClient{
		ConnectFunc: func(ctx context.Context) error {
			return fmt.Errorf("connection refused")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	t.Run("block", func(t *testing.T) {
		_, err := app.BlockContact(context.Background(), "j@s.whatsapp.net")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("unblock", func(t *testing.T) {
		_, err := app.UnblockContact(context.Background(), "j@s.whatsapp.net")
		require.ErrorContains(t, err, "connection refused")
	})
	t.Run("list-blocked", func(t *testing.T) {
		result, err := app.ListBlocked(context.Background())
		require.ErrorContains(t, err, "connection refused")
		require.Nil(t, result)
	})
	t.Run("check", func(t *testing.T) {
		result, err := app.CheckOnWhatsApp(context.Background(), []string{"+1234"})
		require.ErrorContains(t, err, "connection refused")
		require.Nil(t, result)
	})
}
