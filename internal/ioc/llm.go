package ioc

//func InitChatModel(cfg *config.LLMConfig) (model.ToolCallingChatModel, error) {
//	ctx := context.Background()
//
//	// 转换配置格式
//	aiCfg := &llm.Config{
//		APIKey:  cfg.APIKey,
//		Model:   cfg.Model,
//		BaseURL: cfg.BaseURL,
//	}
//
//	m, err := llm.NewChatModel(ctx, aiCfg)
//	if err != nil {
//		return nil, fmt.Errorf("failed to initialize chat model: %w", err)
//	}
//
//	return m, nil
//}
