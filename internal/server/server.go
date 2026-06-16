package server

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewUserService,
	NewChatService,
	NewAgentService,
	NewToolService,
	NewModelService,
	NewPermissionService,
	NewHistoryService,
)
