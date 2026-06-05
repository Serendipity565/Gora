package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/eino"
	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/wired"
)

// chatCmd 启动一个交互式的命令行 Agent 对话循环（REPL），
// 默认使用 default session，输入 :quit / :exit 或 Ctrl+D 退出，:reset 清空当前会话历史。
var chatCmd = &cobra.Command{
	Use:           "chat",
	Short:         "启动交互式命令行 Agent 对话",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runChatCommand,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChatCommand(command *cobra.Command, _ []string) error {
	return runChat(command.Context(), command.OutOrStdout(), command.ErrOrStderr(), os.Stdin, options)
}

func runChat(parent context.Context, out, errOut io.Writer, in io.Reader, opts cliOptions) error {
	if parent == nil {
		parent = context.Background()
	}

	cfg, err := appconfig.Read(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	applyRuntimeOverrides(&cfg, opts)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	forceModel := strings.TrimSpace(opts.Model) != ""
	infra, cleanup, err := wired.InitInfra(parent, cfg)
	if err != nil {
		return fmt.Errorf("初始化基础设施失败: %w", err)
	}
	defer cleanup()

	userID := modelSelectionUserID(opts)
	llmIndex := 0
	if !forceModel {
		llmIndex = restoreModelSelection(parent, out, errOut, infra.DB, userID, eino.DefaultSessionID, cfg)
	}

	chatAgent, err := buildChatAgent(parent, cfg, infra.Registry, llmIndex, nil)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}

	sessionID := eino.DefaultSessionID
	restoreConversationContext(parent, out, errOut, infra.DB, infra.Cache, userID, sessionID, cfg, chatAgent)

	fmt.Fprintf(out, "🤖 Agent: %s (模型: %s)\n", chatAgent.ID(), cfg.LLM[llmIndex].DisplayName(llmIndex))
	fmt.Fprintf(out, "🔧 已加载工具: %d 个\n", len(infra.Registry.List()))
	fmt.Fprintln(out, "输入 :quit / :exit 退出，Ctrl+C 也可中断；:reset 清空当前会话历史。")
	fmt.Fprintln(out, strings.Repeat("─", 60))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		cancel()
	}()

	scanner := bufio.NewScanner(in)
	// 默认 64KB 行缓冲在多行黏贴时容易爆，扩到 1MB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Fprint(out, "你 > ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("读取输入失败: %w", err)
			}
			fmt.Fprintln(out)
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		switch input {
		case ":quit", ":exit":
			fmt.Fprintln(out, "👋 再见")
			return nil
		case ":reset":
			chatAgent.RestoreHistories(map[string][]*schema.Message{})
			fmt.Fprintln(out, "已清空当前会话历史")
			continue
		}

		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "\n已中断")
			return nil
		default:
		}

		assistantBuf := strings.Builder{}
		var gotDone, gotError bool

		fmt.Fprint(out, "\n助手 > ")
		for event := range chatAgent.RunSession(ctx, sessionID, input) {
			switch event.Type {
			case core.EventThinking:
				// 思考事件不打印正文；只用一个轻量提示
			case core.EventToolCall:
				fmt.Fprintf(out, "\n🔧 调用工具 %s\n", event.Content)
			case core.EventToolResult:
				fmt.Fprintf(out, "✔ 工具返回: %s\n助手 > ", truncate(event.Content, 200))
			case core.EventChunk:
				assistantBuf.WriteString(event.Content)
				fmt.Fprint(out, event.Content)
			case core.EventDone:
				gotDone = true
			case core.EventError:
				gotError = true
				fmt.Fprintf(errOut, "\n❌ 错误: %s\n", event.Content)
			}
		}
		fmt.Fprintln(out)

		if gotDone && !gotError {
			persistConversationTurn(parent, errOut, infra.DB, infra.Cache, userID, sessionID, cfg, llmIndex, input, assistantBuf.String(), chatAgent)
		}
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}
