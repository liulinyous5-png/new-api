package model

import (
	"context"
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// ChannelScope restricts dashboard administration, never relay routing.
type ChannelScope struct {
	Mode  string `json:"mode"`
	Types []int  `json:"types"`
}

var ErrInvalidChannelScope = errors.New("Invalid channel visibility scope")

func (scope *ChannelScope) Normalize() error {
	if scope.Mode != "all" && scope.Mode != "selected" {
		return ErrInvalidChannelScope
	}
	if scope.Mode == "all" {
		if len(scope.Types) != 0 {
			return ErrInvalidChannelScope
		}
		scope.Types = []int{}
		return nil
	}
	if len(scope.Types) == 0 || len(scope.Types) > len(constant.ChannelTypeNames) {
		return ErrInvalidChannelScope
	}
	for _, channelType := range scope.Types {
		if _, ok := constant.ChannelTypeNames[channelType]; !ok || channelType <= 0 {
			return ErrInvalidChannelScope
		}
	}
	slices.Sort(scope.Types)
	scope.Types = slices.Compact(scope.Types)
	return nil
}

func (scope ChannelScope) Restricted() bool { return scope.Mode != "all" }

func (scope ChannelScope) Allows(channelType int) bool {
	return scope.Mode == "all" || (scope.Mode == "selected" && slices.Contains(scope.Types, channelType))
}

func (scope ChannelScope) Apply(query *gorm.DB) *gorm.DB {
	if scope.Mode == "all" {
		return query
	}
	if scope.Mode != "selected" || len(scope.Types) == 0 {
		return query.Where("1 = 0")
	}
	return query.Where("type IN ?", scope.Types)
}

func (user *User) ChannelScope() (ChannelScope, error) {
	scope := ChannelScope{Mode: "all", Types: []int{}}
	if user.Role == common.RoleRootUser {
		return scope, nil
	}
	if user.AdminChannelScopeData != "" {
		scope = ChannelScope{}
		if err := common.UnmarshalJsonStr(user.AdminChannelScopeData, &scope); err != nil {
			return ChannelScope{}, ErrInvalidChannelScope
		}
		if err := scope.Normalize(); err != nil {
			return ChannelScope{}, err
		}
	}
	return scope, nil
}

// GetAdminChannelScope reads current authorization state from the primary DB.
// A stale session/PAT snapshot must never restore a revoked channel scope.
func GetAdminChannelScope(userID int) (ChannelScope, error) {
	var user User
	if err := DB.Select("id", "role", "status", "admin_channel_scope").First(&user, userID).Error; err != nil {
		return ChannelScope{}, err
	}
	if user.Status != common.UserStatusEnabled || user.Role < common.RoleAdminUser {
		return ChannelScope{}, errors.New("Unauthorized, insufficient privileges")
	}
	return user.ChannelScope()
}

type channelScopeContextKey struct{}

func WithChannelScope(ctx context.Context, scope ChannelScope) context.Context {
	return context.WithValue(ctx, channelScopeContextKey{}, scope)
}

// ChannelScopeFromContext is used by management handlers after the route guard.
// Internal scheduled work explicitly uses the all scope.
func ChannelScopeFromContext(ctx context.Context) ChannelScope {
	if ctx == nil {
		return ChannelScope{Mode: "all", Types: []int{}}
	}
	if scope, ok := ctx.Value(channelScopeContextKey{}).(ChannelScope); ok {
		return scope
	}
	return ChannelScope{Mode: "all", Types: []int{}}
}

func SetAdminChannelScopeInTx(tx *gorm.DB, userID int, scope ChannelScope) error {
	if err := scope.Normalize(); err != nil {
		return err
	}
	data, err := common.Marshal(scope)
	if err != nil {
		return err
	}
	return tx.Model(&User{}).Where("id = ?", userID).Update("admin_channel_scope", string(data)).Error
}
