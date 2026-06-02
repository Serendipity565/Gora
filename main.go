package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Serendipity565/gora/agent"
	"github.com/Serendipity565/gora/llm"
	"github.com/Serendipity565/gora/tool"
	"github.com/Serendipity565/gora/tool/builtin"
)

func main() {
	fmt.Println("╔══════════════════════════════════╗")
	fmt.Println("║        Gora Agent CLI           ║")
	fmt.Println("║   Goroutine + Agent = Gora      ║")
	fmt.Println("╚══════════════════════════════════╝")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	// 优先使用环境变量，便于本地开发和自动化运行。
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Print("请输入 DeepSeek API Key: ")
		if scanner.Scan() {
			apiKey = strings.TrimSpace(scanner.Text())
		}
		if apiKey == "" {
			fmt.Println("❌ API Key 不能为空")
			os.Exit(1)
		}
	}

	dsConfig := llm.DefaultDeepSeekConfig(apiKey)
	dsModel, err := llm.NewDeepSeekEinoModel(context.Background(), dsConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 DeepSeek Eino 模型失败: %v\n", err)
		os.Exit(1)
	}

	// 注册 Agent 可调用的工具。
	registry := tool.NewRegistry()
	if err := registry.Register(builtin.NewHTTPTool()); err != nil {
		fmt.Fprintf(os.Stderr, "注册工具失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 已加载 %d 个工具\n", len(registry.List()))

	agentConfig := agent.DefaultEinoConfig(dsModel, registry)
	myAgent, err := agent.NewEinoAgent("agent-1", agentConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 Eino Agent 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🤖 %s 已就绪 (模型: %s)\n", myAgent.ID(), dsConfig.Model)
	fmt.Println("输入消息与 Agent 对话，输入 /quit 退出")
	fmt.Println(strings.Repeat("─", 50))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 捕获退出信号，让正在运行的 Agent 有机会感知取消。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n👋 正在退出...")
		cancel()
	}()

	for {
		fmt.Print("\n💬 你: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" {
			fmt.Println("👋 再见！")
			break
		}

		fmt.Printf("\n🤖 %s:\n", myAgent.ID())

		// 消费 Agent 事件流，并按事件类型渲染到命令行。
		events := myAgent.Run(ctx, input)
		for event := range events {
			switch event.Type {
			case agent.EventThinking:
				fmt.Printf("  🧠 %s\n", event.Content)
			case agent.EventToolCall:
				fmt.Printf("  🔧 调用工具: %s\n", event.Content)
				if args, ok := event.Metadata["args"]; ok {
					fmt.Printf("     参数: %s\n", jsonMarshal(args))
				}
			case agent.EventToolResult:
				result := truncateRunes(event.Content, 200)
				fmt.Printf("  📥 工具返回: %s\n", result)
			case agent.EventChunk:
				fmt.Print(event.Content)
			case agent.EventDone:
				fmt.Println()
			case agent.EventError:
				fmt.Printf("\n  ❌ 错误: %s\n", event.Content)
			}
		}

		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
	}
}

// jsonMarshal 把调试信息格式化成 JSON 字符串，失败时退回到 fmt。
func jsonMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// truncateRunes 按字符数截断字符串，避免截断 UTF-8 字节。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
