package commands

import (
	"context"
	"fmt"

	"github.com/vicentereig/whatsapp-cli/internal/types"
)

func (a *App) GroupsList(ctx context.Context) ([]types.GroupInfo, error) {
	if err := a.client.Connect(ctx); err != nil {
		return nil, err
	}

	groups, err := a.client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []types.GroupInfo{}
	}

	return groups, nil
}

func (a *App) GroupsInfo(ctx context.Context, jid string) (*types.GroupInfo, error) {
	if err := a.client.Connect(ctx); err != nil {
		return nil, err
	}

	info, err := a.client.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (a *App) GroupsCreate(ctx context.Context, name string, members []string) (*types.GroupInfo, error) {
	if err := a.client.Connect(ctx); err != nil {
		return nil, err
	}

	info, err := a.client.CreateGroup(ctx, name, members)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (a *App) GroupsInviteLink(ctx context.Context, jid string, reset bool) (InviteLinkResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return InviteLinkResult{}, err
	}

	link, err := a.client.GetGroupInviteLink(ctx, jid, reset)
	if err != nil {
		return InviteLinkResult{}, err
	}

	return InviteLinkResult{JID: jid, Link: link}, nil
}

func (a *App) GroupsJoin(ctx context.Context, link string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	jid, err := a.client.JoinGroupWithLink(ctx, link)
	if err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Joined: true}, nil
}

func (a *App) GroupsLeave(ctx context.Context, jid string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.LeaveGroup(ctx, jid); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Left: true}, nil
}

func (a *App) GroupsAddMembers(ctx context.Context, jid string, members []string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.UpdateGroupParticipants(ctx, jid, members, "add"); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Added: true, Members: members}, nil
}

func (a *App) GroupsRemoveMembers(ctx context.Context, jid string, members []string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.UpdateGroupParticipants(ctx, jid, members, "remove"); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Removed: true, Members: members}, nil
}

func (a *App) GroupsSetName(ctx context.Context, jid, name string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.SetGroupName(ctx, jid, name); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Updated: true, Name: name}, nil
}

func (a *App) GroupsSetDescription(ctx context.Context, jid, description string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.SetGroupDescription(ctx, jid, description); err != nil {
		return JIDResult{}, err
	}

	return JIDResult{JID: jid, Updated: true, Description: description}, nil
}

func (a *App) GroupsSetPhoto(ctx context.Context, jid, imagePath string) (JIDResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return JIDResult{}, err
	}

	if err := a.client.SetGroupPhoto(ctx, jid, imagePath); err != nil {
		return JIDResult{}, fmt.Errorf("setting group photo: %w", err)
	}

	return JIDResult{JID: jid, Updated: true}, nil
}
