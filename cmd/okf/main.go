package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	okf "github.com/agent/okf"
)

// CLI 版本信息
const (
	Version = "1.0.0"
	Usage   = `okf - Open Knowledge Format CLI

Usage:
  okf <command> [options]

Commands:
  init        初始化知识库（从 git 仓库扫描生成）
  update      基于最新提交更新知识库
  lint        检查知识库规范
  show        显示知识库信息
  search      搜索知识库
  hook        安装 git hook 实现自动更新
  version     显示版本信息
  help        显示帮助信息

Options:
  -repo PATH       仓库路径（默认当前目录）
  -dir PATH        知识库目录（默认 .okf/knowledge）
  -verbose         显示详细输出
  -strict          lint 严格模式
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Print(Usage)
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "init", "generate":
		cmdInit(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	case "lint":
		cmdLint(os.Args[2:])
	case "show", "info":
		cmdShow(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	case "hook":
		cmdHook(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("okf version %s\n", Version)
	case "help", "--help", "-h":
		fmt.Print(Usage)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		fmt.Print(Usage)
		os.Exit(1)
	}
}

// -----------------------------------------------------------------------------
// init - 初始化知识库
// -----------------------------------------------------------------------------

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	repoPath := fs.String("repo", "", "仓库路径")
	knowledgeDir := fs.String("dir", ".okf/knowledge", "知识库目录")
	force := fs.Bool("force", false, "覆盖已有文件")
	verbose := fs.Bool("verbose", false, "详细输出")
	noSave := fs.Bool("nosave", false, "只生成不保存")
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	// 检查是否为 git 仓库
	if !okf.IsGitRepo(*repoPath) {
		fmt.Printf("Error: %s is not a git repository\n", *repoPath)
		os.Exit(1)
	}

	config := okf.DefaultGitConfig()
	config.RepoPath = *repoPath
	config.KnowledgeDir = *knowledgeDir

	if *verbose {
		fmt.Printf("Repository: %s\n", config.RepoPath)
		fmt.Printf("Knowledge Dir: %s\n", filepath.Join(config.RepoPath, config.KnowledgeDir))
	}

	start := time.Now()

	fmt.Println("Generating knowledge base from repository...")
	bundle, err := okf.GenerateBundle(config, *force)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	if *noSave {
		fmt.Printf("Generated %d concepts in %v\n", len(bundle.Concepts), elapsed)
		return
	}

	saved, err := okf.SaveKnowledgeBase(bundle, config)
	if err != nil {
		fmt.Printf("Error saving: %v\n", err)
		os.Exit(1)
	}

	outputDir := filepath.Join(config.RepoPath, config.KnowledgeDir)
	fmt.Printf("\n✓ Generated %d concepts (%d saved to disk)\n", len(bundle.Concepts), saved)
	fmt.Printf("✓ Knowledge base saved to: %s\n", outputDir)
	fmt.Printf("✓ Took %v\n", elapsed)

	// Lint 检查
	fmt.Println("\nRunning lint check...")
	lintResult := okf.LintBundle(bundle, okf.DefaultLintConfig())
	if lintResult.HasIssues(false) {
		fmt.Printf("⚠ Lint found %d issues: %d errors, %d warnings, %d infos\n",
			lintResult.Errors+lintResult.Warnings+lintResult.Infos,
			lintResult.Errors, lintResult.Warnings, lintResult.Infos)
	} else {
		fmt.Println("✓ Lint passed (no issues)")
	}

	// 显示统计
	stats := bundle.Stats()
	fmt.Println("\n=== Statistics ===")
	fmt.Printf("Total Concepts: %d\n", stats.TotalConcepts)
	fmt.Printf("Unique Types:   %d\n", stats.UniqueTypes)
	fmt.Printf("Unique Tags:    %d\n", stats.UniqueTags)

	for t, c := range stats.TypeCounts {
		fmt.Printf("  - %s: %d\n", t, c)
	}

	fmt.Printf("\nDone! Your OKF knowledge base is ready at:\n  %s\n", outputDir)
}

// -----------------------------------------------------------------------------
// update - 更新知识库
// -----------------------------------------------------------------------------

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	repoPath := fs.String("repo", "", "仓库路径")
	knowledgeDir := fs.String("dir", ".okf/knowledge", "知识库目录")
	full := fs.Bool("full", false, "完整重新生成")
	verbose := fs.Bool("verbose", false, "详细输出")
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	config := okf.DefaultGitConfig()
	config.RepoPath = *repoPath
	config.KnowledgeDir = *knowledgeDir

	if *full {
		fmt.Println("Running full regeneration...")
		bundle, err := okf.GenerateBundle(config, true)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		saved, err := okf.SaveKnowledgeBase(bundle, config)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Full regeneration complete: %d concepts saved\n", saved)
		return
	}

	start := time.Now()
	fmt.Println("Updating knowledge base from latest commit...")

	bundle, updated, err := okf.UpdateFromLastCommit(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if bundle == nil || len(bundle.Concepts) == 0 {
		fmt.Println("No changes detected, nothing to update.")
		return
	}

	// 保存增量更新
	outputDir := filepath.Join(config.RepoPath, config.KnowledgeDir)
	os.MkdirAll(outputDir, 0755)
	for _, concept := range bundle.Concepts {
		relPath := concept.FilePath
		if !strings.HasSuffix(relPath, ".md") {
			relPath = relPath + ".md"
		}
		fullPath := filepath.Join(outputDir, relPath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		data, _ := okf.SerializeConcept(concept, true)
		os.WriteFile(fullPath, []byte(data), 0644)
	}

	elapsed := time.Since(start)

	fmt.Printf("✓ Updated %d concepts in %v\n", len(bundle.Concepts), elapsed)
	if *verbose {
		fmt.Println("\nChanged files:")
		for _, p := range updated {
			fmt.Printf("  - %s\n", p)
		}
	}
}

// -----------------------------------------------------------------------------
// lint - 检查知识库规范
// -----------------------------------------------------------------------------

func cmdLint(args []string) {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	path := fs.String("path", "", "知识库路径")
	strict := fs.Bool("strict", false, "严格模式（警告也失败）")
	verbose := fs.Bool("verbose", false, "显示所有问题，包括 info")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	var bundle *okf.KnowledgeBundle
	var err error

	// 检查是否是 bundle 目录
	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, &okf.LoadOptions{Recursive: true})
	} else {
		// 检查是否是 md 文件目录
		if okf.Exists(*path) {
			bundle, err = okf.LoadBundle(*path, &okf.LoadOptions{Recursive: true})
		} else {
			fmt.Printf("Error: path does not exist: %s\n", *path)
			os.Exit(1)
		}
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if bundle == nil || len(bundle.Concepts) == 0 {
		fmt.Println("No concepts found.")
		os.Exit(0)
	}

	config := okf.DefaultLintConfig()
	if *strict {
		config.StrictMode = true
	}

	result := okf.LintBundle(bundle, config)

	fmt.Printf("Linting %d concepts...\n\n", result.ConceptsChecked)

	// 显示问题
	for _, issue := range result.Issues {
		severityIcon := map[okf.Severity]string{
			okf.Error:   "❌",
			okf.Warning: "⚠",
			okf.Info:    "ℹ",
		}[issue.Severity]

		severityFilter := okf.Warning
		if *verbose {
			severityFilter = okf.Info
		}

		if issue.Severity >= severityFilter {
			loc := issue.FilePath
			if issue.Line > 0 {
				loc = fmt.Sprintf("%s:%d", loc, issue.Line)
			}
			fmt.Printf("%s [%s] %s - %s\n", severityIcon, issue.Code, loc, issue.Message)
			if *verbose && issue.Suggestion != "" {
				fmt.Printf("   → %s\n", issue.Suggestion)
			}
		}
	}

	fmt.Printf("\n%s\n", result.Summary())

	if result.HasIssues(*strict) {
		os.Exit(1)
	}
	fmt.Println("✓ All checks passed!")
}

// -----------------------------------------------------------------------------
// show - 显示知识库信息
// -----------------------------------------------------------------------------

func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	path := fs.String("path", "", "知识库路径")
	detail := fs.Bool("detail", false, "显示详细信息")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	var bundle *okf.KnowledgeBundle
	var err error

	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, &okf.LoadOptions{Recursive: true})
	} else {
		bundle, err = okf.LoadBundle(*path, &okf.LoadOptions{Recursive: true})
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Knowledge Bundle: %s\n", bundle.Name)
	fmt.Printf("Root Path: %s\n", bundle.RootPath)
	fmt.Printf("Concepts: %d\n\n", len(bundle.Concepts))

	stats := bundle.Stats()
	fmt.Println("=== Statistics ===")
	fmt.Printf("Total: %d | Types: %d | Tags: %d\n\n",
		stats.TotalConcepts, stats.UniqueTypes, stats.UniqueTags)

	fmt.Println("=== Concepts by Type ===")
	for t, c := range stats.TypeCounts {
		fmt.Printf("  %-15s %d\n", t, c)
	}

	if *detail {
		fmt.Println("\n=== Concepts ===")
		for _, c := range bundle.Concepts {
			fmt.Printf("  [%s] %s - %s\n", c.Type, c.Title, c.Description)
		}
	}
}

// -----------------------------------------------------------------------------
// search - 搜索知识库
// -----------------------------------------------------------------------------

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	path := fs.String("path", "", "知识库路径")
	query := fs.String("q", "", "搜索关键词")
	cType := fs.String("type", "", "按类型过滤")
	tag := fs.String("tag", "", "按标签过滤")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	if *query == "" && *cType == "" && *tag == "" {
		fmt.Println("Error: please specify -q, -type, or -tag")
		os.Exit(1)
	}

	var bundle *okf.KnowledgeBundle
	var err error

	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, &okf.LoadOptions{Recursive: true})
	} else {
		bundle, err = okf.LoadBundle(*path, &okf.LoadOptions{Recursive: true})
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	results := buildQuery(bundle, *query, *cType, *tag)

	if len(results) == 0 {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, c := range results {
		fmt.Printf("%d. [%s] %s\n", i+1, c.Type, c.Title)
		fmt.Printf("   %s\n", c.Description)
		if c.FilePath != "" {
			fmt.Printf("   → %s\n", c.FilePath)
		}
		fmt.Println()
	}
}

func buildQuery(bundle *okf.KnowledgeBundle, text, cType, tag string) []*okf.Concept {
	var results []*okf.Concept

	for _, c := range bundle.Concepts {
		if cType != "" && c.Type != cType {
			continue
		}
		if tag != "" {
			found := false
			for _, t := range c.Tags {
				if t == tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if text != "" {
			lowerText := strings.ToLower(text)
			matched := strings.Contains(strings.ToLower(c.Title), lowerText) ||
				strings.Contains(strings.ToLower(c.Description), lowerText) ||
				strings.Contains(strings.ToLower(c.Content), lowerText)
			if !matched {
				continue
			}
		}
		results = append(results, c)
	}

	return results
}

// -----------------------------------------------------------------------------
// hook - 安装 git hook
// -----------------------------------------------------------------------------

func cmdHook(args []string) {
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	repoPath := fs.String("repo", "", "仓库路径")
	hookType := fs.String("type", "post-commit", "hook 类型: pre-commit, post-commit, pre-push")
	uninstall := fs.Bool("uninstall", false, "卸载 hook")
	force := fs.Bool("force", false, "覆盖已有 hook")
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	root, err := okf.GetRepoRoot(*repoPath)
	if err != nil {
		fmt.Printf("Error: not a git repository: %v\n", err)
		os.Exit(1)
	}

	hookDir := filepath.Join(root, ".git", "hooks")
	hookFile := filepath.Join(hookDir, *hookType)

	if *uninstall {
		if okf.Exists(hookFile) {
			if err := os.Remove(hookFile); err != nil {
				fmt.Printf("Error: failed to remove hook: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Removed %s hook\n", *hookType)
			return
		}
		fmt.Printf("Hook %s not found\n", *hookType)
		return
	}

	// 检查是否已存在
	if okf.Exists(hookFile) && !*force {
		content, _ := os.ReadFile(hookFile)
		if strings.Contains(string(content), "# OKF Hook") {
			fmt.Println("OKF hook already installed. Use -force to overwrite.")
			return
		}
		fmt.Printf("Warning: existing hook found at %s\n", hookFile)
		fmt.Println("Use -force to overwrite, or manually merge the scripts.")
		os.Exit(1)
	}

	os.MkdirAll(hookDir, 0755)

	// 生成 hook 脚本
	script := generateHookScript(*hookType, root)
	if err := os.WriteFile(hookFile, []byte(script), 0755); err != nil {
		fmt.Printf("Error: failed to write hook: %v\n", err)
		os.Exit(1)
	}

	// 使其可执行
	os.Chmod(hookFile, 0755)

	fmt.Printf("✓ Installed %s hook at: %s\n", *hookType, hookFile)
	fmt.Println("  Now every commit will automatically update the OKF knowledge base.")
	fmt.Printf("  Knowledge base location: %s/.okf/knowledge/\n", root)
}

// generateHookScript 生成 hook 脚本
func generateHookScript(hookType, repoRoot string) string {
	shebang := "#!/bin/bash\n"
	header := "\n# OKF Hook - Automated Knowledge Base Update\n# Generated by okf CLI v" + Version + "\n\n"

	var body string
	switch hookType {
	case "post-commit":
		body = `# 更新知识库（基于最新提交的变更）
echo "[OKF] Updating knowledge base from commit..."
cd "$(git rev-parse --show-toplevel)"
if command -v okf &> /dev/null; then
    okf update -verbose
    # 将更新加入暂存区（可选，注释掉则不会自动提交）
    # git add .okf/knowledge/
elif command -v go &> /dev/null; then
    # 尝试用 go run 执行（如果 okf 命令不在 PATH）
    if [ -f "go.mod" ] && grep -q "okf" go.mod 2>/dev/null; then
        go run . update -verbose 2>/dev/null || true
    fi
else
    echo "[OKF] Warning: okf command not found. Install with: go install github.com/agent/okf@latest"
fi

exit 0
`
	case "pre-commit":
		body = `# 先 lint 检查（如果存在知识库）
OKF_DIR="$(git rev-parse --show-toplevel)/.okf/knowledge"
if [ -d "$OKF_DIR" ]; then
    echo "[OKF] Linting knowledge base before commit..."
    if command -v okf &> /dev/null; then
        cd "$(git rev-parse --show-toplevel)"
        if ! okf lint; then
            echo ""
            echo "[OKF] ❌ Lint failed. Please fix the issues before committing."
            echo "       Run: okf lint -verbose  for more details."
            exit 1
        fi
        echo "[OKF] ✓ Lint passed"
    fi
fi

exit 0
`
	case "pre-push":
		body = `# 推送前生成完整知识库（确保最新）
echo "[OKF] Preparing knowledge base for push..."
cd "$(git rev-parse --show-toplevel)"
if command -v okf &> /dev/null; then
    okf init -force 2>/dev/null || true
fi

exit 0
`
	default:
		body = "# Unsupported hook type\nexit 0\n"
	}

	return shebang + header + body
}
