package authz

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type channelTaskActor struct {
	UserID     int
	Permission Permission
}
type channelTaskActorKey struct{}

// ChannelTaskContext carries the human actor through asynchronous work. Zero is
// reserved for internal scheduled jobs; clients never supply this value.
func ChannelTaskContext(ctx context.Context, userID int, permission Permission) (context.Context, error) {
	if userID == 0 {
		return ctx, nil
	}
	var user model.User
	if err := model.DB.Select("id", "role", "status", "admin_channel_scope").First(&user, userID).Error; err != nil {
		return ctx, err
	}
	if user.Status != common.UserStatusEnabled || !Can(user.Id, user.Role, permission) {
		return ctx, errors.New("Unauthorized, insufficient privileges")
	}
	scope, err := user.ChannelScope()
	if err != nil {
		return ctx, err
	}
	ctx = model.WithChannelScope(ctx, scope)
	return context.WithValue(ctx, channelTaskActorKey{}, channelTaskActor{userID, permission}), nil
}

// ChannelTaskAllows rechecks the actor and current channel type at execution,
// so queued work cannot outlive a revocation or a channel type change.
func ChannelTaskAllows(ctx context.Context, channel *model.Channel) bool {
	if ctx == nil {
		return true
	}
	actor, ok := ctx.Value(channelTaskActorKey{}).(channelTaskActor)
	if !ok {
		return true
	}
	current, err := ChannelTaskContext(ctx, actor.UserID, actor.Permission)
	if err != nil {
		return false
	}
	var stored model.Channel
	if err := model.DB.Select("id", "type").First(&stored, channel.Id).Error; err != nil {
		return false
	}
	return stored.Type == channel.Type && model.ChannelScopeFromContext(current).Allows(stored.Type)
}
