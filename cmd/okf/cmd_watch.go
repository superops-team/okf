package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	okf "github.com/superops-team/okf/pkg/okf"
)

// cmdWatch 启动文件监听守护进程
// 用法: okf watch [options]
func cmdWatch(args []string) int {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf watch [options]")
		fmt.Println("")
		fmt.Println("Watch source directories for changes and automatically import / merge into knowledge base.")
		fmt.Println("Requires a .watch.yaml config file in the working directory or specified via -config.")
		fmt.Println("")
		fmt.Println("Options:")
		flags.PrintDefaults()
	}

	var (
		configPath = flags.String("config", "", "Path to watch config file or directory (default: current dir)")
		dirFlag    = flags.String("dir", "", "Override knowledge base directory (overrides config)")
		silent     = flags.Bool("silent", false, "Suppress informational output")
	)

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	// 默认：当前目录
	cfgDir := *configPath
	if cfgDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		cfgDir = wd
	}

	cfg, err := okf.LoadWatchConfig(cfgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load watch config: %v\n", err)
		return 1
	}

	// 允许 CLI 覆盖 knowledge dir
	if *dirFlag != "" {
		// 解析为绝对路径
		absDir, err := filepath.Abs(*dirFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		cfg.KnowledgeDir = absDir
	}

	if !*silent {
		fmt.Printf("Watching %d rules...\n", len(cfg.Rules))
		for _, r := range cfg.Rules {
			fmt.Printf("  [%s] %s → %s (patterns: %v)\n", r.Strategy, r.Name, r.Sources, r.Patterns)
		}
		fmt.Println("Press Ctrl+C to stop.")
	}

	daemon, err := okf.NewWatchDaemon(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// 监听系统信号以优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if !*silent {
			fmt.Println("\nShutting down...")
		}
		cancel()
	}()

	if err := daemon.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
