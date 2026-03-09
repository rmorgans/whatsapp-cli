package commands

import (
	"context"
	"encoding/json"
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

	result := app.BlockContact(context.Background(), "5511999999999@s.whatsapp.net")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["blocked"])
	require.Equal(t, "5511999999999@s.whatsapp.net", data["jid"])

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

	result := app.BlockContact(context.Background(), "bad@s.whatsapp.net")

	resp := parseResponse(t, result)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	require.Contains(t, *resp.Error, "server error")
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

	result := app.UnblockContact(context.Background(), "5511999999999@s.whatsapp.net")

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var data map[string]interface{}
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	require.Equal(t, true, data["unblocked"])
	require.Equal(t, "5511999999999@s.whatsapp.net", data["jid"])

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

	result := app.ListBlocked(context.Background())

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var jids []string
	err := json.Unmarshal(resp.Data, &jids)
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

	result := app.ListBlocked(context.Background())

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var jids []string
	err := json.Unmarshal(resp.Data, &jids)
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

	result := app.ListBlocked(context.Background())

	resp := parseResponse(t, result)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	require.Contains(t, *resp.Error, "network error")
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

	result := app.CheckOnWhatsApp(context.Background(), []string{"+5511999999999", "+5511000000000"})

	resp := parseResponse(t, result)
	require.True(t, resp.Success, "should succeed: %v", resp.Error)

	var results []types.IsOnWhatsAppResponse
	err := json.Unmarshal(resp.Data, &results)
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

	result := app.CheckOnWhatsApp(context.Background(), []string{"+5511999999999"})

	resp := parseResponse(t, result)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	require.Contains(t, *resp.Error, "rate limited")
}

func TestContactsConnectError(t *testing.T) {
	mockClient := &MockWAClient{
		ConnectFunc: func(ctx context.Context) error {
			return fmt.Errorf("connection refused")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	tests := []struct {
		name string
		fn   func() string
	}{
		{"block", func() string { return app.BlockContact(context.Background(), "j@s.whatsapp.net") }},
		{"unblock", func() string { return app.UnblockContact(context.Background(), "j@s.whatsapp.net") }},
		{"list-blocked", func() string { return app.ListBlocked(context.Background()) }},
		{"check", func() string { return app.CheckOnWhatsApp(context.Background(), []string{"+1234"}) }},
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
