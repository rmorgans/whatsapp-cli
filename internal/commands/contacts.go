package commands

import (
	"context"

	"github.com/vicentereig/whatsapp-cli/internal/types"
)

func (a *App) BlockContact(ctx context.Context, jid string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.UpdateBlocklist(ctx, jid, "block"); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Blocked: true}, nil
}

func (a *App) UnblockContact(ctx context.Context, jid string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.UpdateBlocklist(ctx, jid, "unblock"); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Unblocked: true}, nil
}

func (a *App) ListBlocked(ctx context.Context) ([]string, error) {
	if err := a.client.Connect(ctx); err != nil {
		return nil, err
	}

	jids, err := a.client.GetBlocklist(ctx)
	if err != nil {
		return nil, err
	}
	if jids == nil {
		jids = []string{}
	}

	return jids, nil
}

func (a *App) CheckOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if err := a.client.Connect(ctx); err != nil {
		return nil, err
	}

	results, err := a.client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []types.IsOnWhatsAppResponse{}
	}

	return results, nil
}
