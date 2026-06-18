package main

import (
	"flag"
	"fmt"
	"os"

	okf "github.com/superops-team/okf/pkg/okf"
)

// cmdConfig handles the "okf config" command.
func cmdConfig(args []string) int {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf config <subcommand> [options]")
		fmt.Println("")
		fmt.Println("Manage OKF configuration.")
		fmt.Println("")
		fmt.Println("Subcommands:")
		fmt.Println("  list              Show all configuration")
		fmt.Println("  get <key>         Get specific configuration value")
		fmt.Println("  set <key> <value> Set configuration value")
		fmt.Println("")
		fmt.Println("Keys:")
		fmt.Println("  knowledge_dir     Knowledge base directory path")
	}

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	subArgs := flags.Args()
	if len(subArgs) < 1 {
		flags.Usage()
		return 1
	}

	subcommand := subArgs[0]

	switch subcommand {
	case "list":
		return configList()
	case "get":
		if len(subArgs) < 2 {
			fmt.Fprintln(os.Stderr, "Error: missing key argument")
			return 1
		}
		return configGet(subArgs[1])
	case "set":
		if len(subArgs) < 3 {
			fmt.Fprintln(os.Stderr, "Error: missing key or value argument")
			return 1
		}
		return configSet(subArgs[1], subArgs[2])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown subcommand: %s\n", subcommand)
		flags.Usage()
		return 1
	}
}

func configList() int {
	// Get config file path
	configPath := getConfigPath()

	// Load config
	cfg, err := okf.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		return 1
	}

	// Resolve knowledge directory
	resolved, err := okf.ResolveKnowledgePaths(okf.ResolveKnowledgePathsOptions{ReadOnly: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve knowledge directory: %v\n", err)
		return 1
	}

	fmt.Println("OKF Configuration")
	fmt.Println("==================")
	fmt.Println("")

	// Show knowledge_dir
	fmt.Printf("knowledge_dir: %s\n", resolved.WritePath)
	if cfg != nil && cfg.KnowledgeDir != "" {
		fmt.Printf("  (from config: %s)\n", cfg.KnowledgeDir)
	} else if os.Getenv("OKF_KNOWLEDGE_DIR") != "" {
		fmt.Printf("  (from env: %s)\n", os.Getenv("OKF_KNOWLEDGE_DIR"))
	} else {
		fmt.Printf("  (source: %s)\n", resolved.WriteSource)
	}
	if len(cfg.KnowledgePaths) > 0 {
		fmt.Println("knowledge_paths:")
		for _, path := range cfg.KnowledgePaths {
			fmt.Printf("  - %s\n", path)
		}
	}
	fmt.Println("read_paths:")
	for _, path := range resolved.ReadPaths {
		fmt.Printf("  - [%d] %s (source: %s)\n", path.Rank, path.Path, path.Source)
	}
	fmt.Println("")

	// Show platform default
	fmt.Printf("platform_default: %s\n", okf.GetPlatformDefault())
	fmt.Printf("config_file: %s\n", configPath)

	return 0
}

func configGet(key string) int {
	switch key {
	case "knowledge_dir":
		resolved, err := okf.ResolveKnowledgePaths(okf.ResolveKnowledgePathsOptions{ReadOnly: true})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("%s (source: %s)\n", resolved.WritePath, resolved.WriteSource)
	case "platform_default":
		fmt.Println(okf.GetPlatformDefault())
	case "config_path":
		fmt.Println(getConfigPath())
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config key: %s\n", key)
		return 1
	}

	return 0
}

func configSet(key, value string) int {
	configPath := getConfigPath()

	// Load existing config or create new one
	cfg, err := okf.LoadConfig(configPath)
	if err != nil {
		cfg = &okf.Config{}
	}

	switch key {
	case "knowledge_dir":
		cfg.KnowledgeDir = value
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config key: %s\n", key)
		return 1
	}

	// Save config
	if err := okf.SaveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save config: %v\n", err)
		return 1
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return 0
}

func getConfigPath() string {
	return okf.GetDefaultConfigPath()
}
