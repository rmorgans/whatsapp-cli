package commands

import (
	"context"
	"fmt"

	"github.com/vicentereig/whatsapp-cli/internal/output"
	"github.com/vicentereig/whatsapp-cli/internal/types"
)

func (a *App) GroupsList(ctx context.Context) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	groups, err := a.client.GetJoinedGroups(ctx)
	if err != nil {
		return output.Error(err)
	}
	if groups == nil {
		groups = []types.GroupInfo{}
	}

	return output.Success(groups)
}

func (a *App) GroupsInfo(ctx context.Context, jid string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	info, err := a.client.GetGroupInfo(ctx, jid)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(info)
}

func (a *App) GroupsCreate(ctx context.Context, name string, members []string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	info, err := a.client.CreateGroup(ctx, name, members)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(info)
}

func (a *App) GroupsInviteLink(ctx context.Context, jid string, reset bool) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	link, err := a.client.GetGroupInviteLink(ctx, jid, reset)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"jid":  jid,
		"link": link,
	})
}

func (a *App) GroupsJoin(ctx context.Context, link string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	jid, err := a.client.JoinGroupWithLink(ctx, link)
	if err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"joined": true,
		"jid":    jid,
	})
}

func (a *App) GroupsLeave(ctx context.Context, jid string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.LeaveGroup(ctx, jid); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"left": true,
		"jid":  jid,
	})
}

func (a *App) GroupsAddMembers(ctx context.Context, jid string, members []string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.UpdateGroupParticipants(ctx, jid, members, "add"); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"added":   true,
		"jid":     jid,
		"members": members,
	})
}

func (a *App) GroupsRemoveMembers(ctx context.Context, jid string, members []string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.UpdateGroupParticipants(ctx, jid, members, "remove"); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"removed": true,
		"jid":     jid,
		"members": members,
	})
}

func (a *App) GroupsSetName(ctx context.Context, jid, name string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.SetGroupName(ctx, jid, name); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"updated": true,
		"jid":     jid,
		"name":    name,
	})
}

func (a *App) GroupsSetDescription(ctx context.Context, jid, description string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.SetGroupDescription(ctx, jid, description); err != nil {
		return output.Error(err)
	}

	return output.Success(map[string]interface{}{
		"updated":     true,
		"jid":         jid,
		"description": description,
	})
}

func (a *App) GroupsSetPhoto(ctx context.Context, jid, imagePath string) string {
	if err := a.client.Connect(ctx); err != nil {
		return output.Error(err)
	}

	if err := a.client.SetGroupPhoto(ctx, jid, imagePath); err != nil {
		return output.Error(fmt.Errorf("setting group photo: %w", err))
	}

	return output.Success(map[string]interface{}{
		"updated": true,
		"jid":     jid,
	})
}
