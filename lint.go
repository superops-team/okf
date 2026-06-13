package okf

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Severity 表示 lint 警告的严重程度
type Severity int

const (
	// Info 信息级警告
	Info Severity = iota
	// Warning 警告级
	Warning
	// Error 错误级
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "INFO"
	case Warning:
		return "WARNING"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LintIssue 表示一个 lint 检查结果
type LintIssue struct {
	FilePath    string
	Line        int
	Severity    Severity
	Code        string
	Message     string
	Suggestion  string
}

func (i LintIssue) String() string {
	loc := i.FilePath
	if i.Line > 0 {
		loc = fmt.Sprintf("%s:%d", i.FilePath, i.Line)
	}
	return fmt.Sprintf("[%s] %s %s - %s", i.Severity, loc, i.Code, i.Message)
}

// LintRule 定义一个 lint 规则
type LintRule struct {
	Code        string
	Description string
	Severity    Severity
	Check       func(*Concept) []LintIssue
}

// LintConfig lint 配置
type LintConfig struct {
	// MaxLineLength 内容行最大长度，默认 120
	MaxLineLength int

	// MinDescriptionLength 描述最小长度
	MinDescriptionLength int

	// RequiredTags 必须包含的标签
	RequiredTags []string

	// IgnoredPaths 忽略的文件路径模式
	IgnoredPaths []string

	// StrictMode 是否启用严格模式（Warning 也报错）
	StrictMode bool
}

// DefaultLintConfig 返回默认 lint 配置
func DefaultLintConfig() *LintConfig {
	return &LintConfig{
		MaxLineLength:        240,
		MinDescriptionLength: 10,
		StrictMode:           false,
	}
}

// LintResult lint 完整结果
type LintResult struct {
	ConceptsChecked int
	Issues          []LintIssue
	Errors          int
	Warnings        int
	Infos           int
	Duration        time.Duration
}

// HasErrors 是否有错误
func (r *LintResult) HasErrors() bool {
	return r.Errors > 0
}

// HasIssues 是否有问题（严格模式下包含警告）
func (r *LintResult) HasIssues(strict bool) bool {
	if strict {
		return r.Errors > 0 || r.Warnings > 0
	}
	return r.Errors > 0
}

// Summary 返回结果摘要
func (r *LintResult) Summary() string {
	return fmt.Sprintf("Checked %d concepts: %d errors, %d warnings, %d infos (took %v)",
		r.ConceptsChecked, r.Errors, r.Warnings, r.Infos, r.Duration)
}

// -----------------------------------------------------------------------------
// Lint 规则定义
// -----------------------------------------------------------------------------

func getLintRules(config *LintConfig) []LintRule {
	return []LintRule{
		{
			Code:        "OKF001",
			Description: "Required field 'type' must not be empty",
			Severity:    Error,
			Check: func(c *Concept) []LintIssue {
				if strings.TrimSpace(c.Type) == "" {
					return []LintIssue{{
						Code:       "OKF001",
						Severity:   Error,
						Message:    "'type' field is required and must not be empty",
						Suggestion: "Set type to one of: table, api, metric, concept, system, component, service",
						FilePath:   c.FilePath,
						Line:       1,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF002",
			Description: "Required field 'title' must not be empty",
			Severity:    Error,
			Check: func(c *Concept) []LintIssue {
				if strings.TrimSpace(c.Title) == "" {
					return []LintIssue{{
						Code:       "OKF002",
						Severity:   Error,
						Message:    "'title' field is required and must not be empty",
						Suggestion: "Provide a concise title (1-60 chars recommended)",
						FilePath:   c.FilePath,
						Line:       1,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF003",
			Description: "'description' should provide meaningful context",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				if len(strings.TrimSpace(c.Description)) < config.MinDescriptionLength {
					return []LintIssue{{
						Code:       "OKF003",
						Severity:   Warning,
						Message:    fmt.Sprintf("'description' should be at least %d characters (got %d)", config.MinDescriptionLength, len(strings.TrimSpace(c.Description))),
						Suggestion: "Add a brief description explaining what this concept represents",
						FilePath:   c.FilePath,
						Line:       3,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF004",
			Description: "Type should use lowercase alphanumeric characters only",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*$`, c.Type)
				if c.Type != "" && !matched {
					return []LintIssue{{
						Code:       "OKF004",
						Severity:   Warning,
						Message:    fmt.Sprintf("'type' '%s' should use lowercase alphanumeric characters only", c.Type),
						Suggestion: "Use lowercase letters, digits, and underscores only",
						FilePath:   c.FilePath,
						Line:       1,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF005",
			Description: "Timestamp should be in ISO 8601 format",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				if c.Timestamp == "" {
					return []LintIssue{{
						Code:       "OKF005",
						Severity:   Warning,
						Message:    "'timestamp' is recommended but missing",
						Suggestion: "Set timestamp to the creation/update time (ISO 8601 format: YYYY-MM-DDTHH:MM:SSZ)",
						FilePath:   c.FilePath,
						Line:       6,
					}}
				}
				// Try to parse common formats
				formats := []string{
					time.RFC3339,
					"2006-01-02T15:04:05Z",
					"2006-01-02T15:04:05",
					"2006-01-02",
				}
				valid := false
				for _, f := range formats {
					if _, err := time.Parse(f, c.Timestamp); err == nil {
						valid = true
						break
					}
				}
				if c.Timestamp != "" && !valid {
					return []LintIssue{{
						Code:       "OKF005",
						Severity:   Warning,
						Message:    fmt.Sprintf("'timestamp' '%s' is not in a valid ISO 8601 format", c.Timestamp),
						Suggestion: "Use ISO 8601 format: 2024-01-15T10:30:00Z",
						FilePath:   c.FilePath,
						Line:       6,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF006",
			Description: "Tags should be lowercase and consistent",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				var issues []LintIssue
				for _, tag := range c.Tags {
					if strings.ToLower(tag) != tag {
						issues = append(issues, LintIssue{
							Code:       "OKF006",
							Severity:   Warning,
							Message:    fmt.Sprintf("Tag '%s' should be lowercase", tag),
							Suggestion: "Use lowercase tags for consistency (e.g., 'production' not 'Production')",
							FilePath:   c.FilePath,
							Line:       5,
						})
					}
					if strings.Contains(tag, " ") {
						issues = append(issues, LintIssue{
							Code:       "OKF006",
							Severity:   Warning,
							Message:    fmt.Sprintf("Tag '%s' should not contain spaces", tag),
							Suggestion: "Use hyphens or underscores instead of spaces",
							FilePath:   c.FilePath,
							Line:       5,
						})
					}
				}
				return issues
			},
		},
		{
			Code:        "OKF007",
			Description: "Content body should not be empty",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				if len(strings.TrimSpace(c.Content)) == 0 {
					return []LintIssue{{
						Code:       "OKF007",
						Severity:   Warning,
						Message:    "Content body is empty",
						Suggestion: "Add markdown content explaining the concept in detail",
						FilePath:   c.FilePath,
						Line:       8,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF008",
			Description: "Title matches filename convention",
			Severity:    Info,
			Check: func(c *Concept) []LintIssue {
				filename := filepath.Base(c.FilePath)
				filename = strings.TrimSuffix(filename, ".md")

				// Convert title to filename-like format
				expectedFromTitle := strings.ReplaceAll(strings.ToLower(c.Title), " ", "_")
				expectedFromTitle = sanitizeFilename(expectedFromTitle)

				// Normalize both for comparison
				normFilename := strings.ToLower(filename)

				if normFilename != strings.ToLower(expectedFromTitle) {
					return []LintIssue{{
						Code:       "OKF008",
						Severity:   Info,
						Message:    fmt.Sprintf("Filename '%s' may not match title '%s'", filename, c.Title),
						Suggestion: fmt.Sprintf("Consider naming the file '%s.md' for better discoverability", sanitizeFilename(c.Title)),
						FilePath:   c.FilePath,
						Line:       0,
					}}
				}
				return nil
			},
		},
		{
			Code:        "OKF009",
			Description: "Long lines in content",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				var issues []LintIssue
				lines := strings.Split(c.Content, "\n")
				for i, line := range lines {
					if len(line) > config.MaxLineLength {
						issues = append(issues, LintIssue{
							Code:       "OKF009",
							Severity:   Warning,
							Message:    fmt.Sprintf("Line %d exceeds %d characters (got %d)", i+1, config.MaxLineLength, len(line)),
							Suggestion: fmt.Sprintf("Wrap lines to %d characters for better readability", config.MaxLineLength),
							FilePath:   c.FilePath,
							Line:       8 + i,
						})
						if len(issues) > 5 {
							break // Don't report too many line-length issues
						}
					}
				}
				return issues
			},
		},
		{
			Code:        "OKF010",
			Description: "Duplicate tags",
			Severity:    Warning,
			Check: func(c *Concept) []LintIssue {
				seen := make(map[string]bool)
				for _, tag := range c.Tags {
					if seen[tag] {
						return []LintIssue{{
							Code:       "OKF010",
							Severity:   Warning,
							Message:    fmt.Sprintf("Duplicate tag '%s'", tag),
							Suggestion: "Remove duplicate tags",
							FilePath:   c.FilePath,
							Line:       5,
						}}
					}
					seen[tag] = true
				}
				return nil
			},
		},
		{
			Code:        "OKF011",
			Description: "Required tags check",
			Severity:    Error,
			Check: func(c *Concept) []LintIssue {
				if len(config.RequiredTags) == 0 {
					return nil
				}
				tagSet := make(map[string]bool)
				for _, t := range c.Tags {
					tagSet[strings.ToLower(t)] = true
				}
				var issues []LintIssue
				for _, required := range config.RequiredTags {
					if !tagSet[strings.ToLower(required)] {
						issues = append(issues, LintIssue{
							Code:       "OKF011",
							Severity:   Error,
							Message:    fmt.Sprintf("Missing required tag '%s'", required),
							Suggestion: fmt.Sprintf("Add '%s' to the tags list", required),
							FilePath:   c.FilePath,
							Line:       5,
						})
					}
				}
				return issues
			},
		},
		{
			Code:        "OKF012",
			Description: "Resource field should reference actual resource",
			Severity:    Info,
			Check: func(c *Concept) []LintIssue {
				if c.Resource == "" && c.Type == "table" {
					return []LintIssue{{
						Code:       "OKF012",
						Severity:   Warning,
						Message:    "Table concept should have a 'resource' field referencing the actual table location",
						Suggestion: "e.g., 'bigquery.project.dataset.table' or 'postgres://db:5432/db/schema.table'",
						FilePath:   c.FilePath,
						Line:       4,
					}}
				}
				if c.Resource == "" && c.Type == "api" {
					return []LintIssue{{
						Code:       "OKF012",
						Severity:   Warning,
						Message:    "API concept should have a 'resource' field with the API endpoint",
						Suggestion: "e.g., 'https://api.example.com/v1/users'",
						FilePath:   c.FilePath,
						Line:       4,
					}}
				}
				return nil
			},
		},
	}
}

// LintConcept 检查单个 concept
func LintConcept(c *Concept, config *LintConfig) []LintIssue {
	if config == nil {
		config = DefaultLintConfig()
	}

	var allIssues []LintIssue
	rules := getLintRules(config)

	for _, rule := range rules {
		if rule.Check == nil {
			continue
		}
		issues := rule.Check(c)
		for _, issue := range issues {
			// 确保 FilePath 设置正确
			if issue.FilePath == "" {
				issue.FilePath = c.FilePath
			}
			allIssues = append(allIssues, issue)
		}
	}

	return allIssues
}

// LintBundle 检查整个 bundle
func LintBundle(b *KnowledgeBundle, config *LintConfig) *LintResult {
	start := time.Now()

	if config == nil {
		config = DefaultLintConfig()
	}

	result := &LintResult{
		ConceptsChecked: len(b.Concepts),
	}

	// Track duplicates by title
	titleCounts := make(map[string]int)
	for _, c := range b.Concepts {
		titleCounts[c.Title]++
	}

	for _, c := range b.Concepts {
		// Check individual concept rules
		issues := LintConcept(c, config)
		result.Issues = append(result.Issues, issues...)

		// Check for duplicate titles
		if titleCounts[c.Title] > 1 {
			result.Issues = append(result.Issues, LintIssue{
				FilePath:   c.FilePath,
				Severity:   Warning,
				Code:       "OKF013",
				Message:    fmt.Sprintf("Duplicate title '%s' (appears %d times)", c.Title, titleCounts[c.Title]),
				Suggestion: "Each concept should have a unique title",
			})
		}
	}

	// Count severity
	for _, issue := range result.Issues {
		switch issue.Severity {
		case Error:
			result.Errors++
		case Warning:
			result.Warnings++
		case Info:
			result.Infos++
		}
	}

	result.Duration = time.Since(start)
	return result
}

// LintFiles 直接检查文件列表
func LintFiles(files []string, config *LintConfig) (*LintResult, error) {
	bundle := &KnowledgeBundle{}
	var loadedFiles []string

	for _, file := range files {
		c, err := ParseConcept(file)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", file, err)
		}
		c.FilePath = file
		bundle.Concepts = append(bundle.Concepts, c)
		loadedFiles = append(loadedFiles, file)
	}

	_ = loadedFiles
	return LintBundle(bundle, config), nil
}
