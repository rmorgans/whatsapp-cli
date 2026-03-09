package commands

import (
	"context"

	"github.com/vicentereig/whatsapp-cli/internal/output"
)

func (a *App) BlockContact(ctx context.Context, jid string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.UpdateBlocklist(ctx, jid, "block"); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"blocked": true,
		"jid":     jid,
	})
}

func (a *App) UnblockContact(ctx context.Context, jid string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.UpdateBlocklist(ctx, jid, "unblock"); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"unblocked": true,
		"jid":       jid,
	})
}

func (a *App) ListBlocked(ctx context.Context) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	jids, err := a.client.GetBlocklist(ctx)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(jids)
}

func (a *App) CheckOnWhatsApp(ctx context.Context, phones []string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	results, err := a.client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(results)
}
