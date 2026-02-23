package project

// manifestMap maps manifest filenames to their primary language.
var manifestMap = map[string]string{
	"go.mod":           "go",
	"go.sum":           "go",
	"package.json":     "javascript",
	"tsconfig.json":    "typescript",
	"pyproject.toml":   "python",
	"setup.py":         "python",
	"requirements.txt": "python",
	"Pipfile":          "python",
	"Cargo.toml":       "rust",
	"pom.xml":          "java",
	"build.gradle":     "java",
	"Gemfile":          "ruby",
	"composer.json":    "php",
	"mix.exs":          "elixir",
	"Package.swift":    "swift",
	"Makefile":         "make",
	"CMakeLists.txt":   "cpp",
	"Dockerfile":       "docker",
}

// frameworkRule defines how to detect a framework from a manifest file's content.
type frameworkRule struct {
	Manifest  string // filename to read
	Substring string // string to search for in the file
	Framework string // framework name to report
}

// frameworkDetectors maps language → rules for detecting frameworks.
var frameworkDetectors = map[string][]frameworkRule{
	"go": {
		{Manifest: "go.mod", Substring: "github.com/go-chi", Framework: "chi"},
		{Manifest: "go.mod", Substring: "github.com/gin-gonic/gin", Framework: "gin"},
		{Manifest: "go.mod", Substring: "github.com/labstack/echo", Framework: "echo"},
		{Manifest: "go.mod", Substring: "github.com/gofiber/fiber", Framework: "fiber"},
	},
	"javascript": {
		{Manifest: "package.json", Substring: "\"react\"", Framework: "react"},
		{Manifest: "package.json", Substring: "\"vue\"", Framework: "vue"},
		{Manifest: "package.json", Substring: "\"svelte\"", Framework: "svelte"},
		{Manifest: "package.json", Substring: "\"solid-js\"", Framework: "solidjs"},
		{Manifest: "package.json", Substring: "\"next\"", Framework: "next"},
		{Manifest: "package.json", Substring: "\"nuxt\"", Framework: "nuxt"},
		{Manifest: "package.json", Substring: "\"express\"", Framework: "express"},
		{Manifest: "package.json", Substring: "\"@nestjs/core\"", Framework: "nestjs"},
	},
	"typescript": {
		{Manifest: "package.json", Substring: "\"react\"", Framework: "react"},
		{Manifest: "package.json", Substring: "\"vue\"", Framework: "vue"},
		{Manifest: "package.json", Substring: "\"svelte\"", Framework: "svelte"},
		{Manifest: "package.json", Substring: "\"solid-js\"", Framework: "solidjs"},
		{Manifest: "package.json", Substring: "\"next\"", Framework: "next"},
		{Manifest: "package.json", Substring: "\"@nestjs/core\"", Framework: "nestjs"},
		{Manifest: "package.json", Substring: "\"@angular/core\"", Framework: "angular"},
	},
	"python": {
		{Manifest: "pyproject.toml", Substring: "django", Framework: "django"},
		{Manifest: "pyproject.toml", Substring: "flask", Framework: "flask"},
		{Manifest: "pyproject.toml", Substring: "fastapi", Framework: "fastapi"},
		{Manifest: "requirements.txt", Substring: "django", Framework: "django"},
		{Manifest: "requirements.txt", Substring: "flask", Framework: "flask"},
		{Manifest: "requirements.txt", Substring: "fastapi", Framework: "fastapi"},
	},
	"rust": {
		{Manifest: "Cargo.toml", Substring: "actix-web", Framework: "actix"},
		{Manifest: "Cargo.toml", Substring: "axum", Framework: "axum"},
		{Manifest: "Cargo.toml", Substring: "rocket", Framework: "rocket"},
	},
	"java": {
		{Manifest: "pom.xml", Substring: "spring-boot", Framework: "spring-boot"},
		{Manifest: "build.gradle", Substring: "spring-boot", Framework: "spring-boot"},
	},
	"ruby": {
		{Manifest: "Gemfile", Substring: "rails", Framework: "rails"},
		{Manifest: "Gemfile", Substring: "sinatra", Framework: "sinatra"},
	},
	"php": {
		{Manifest: "composer.json", Substring: "laravel", Framework: "laravel"},
		{Manifest: "composer.json", Substring: "symfony", Framework: "symfony"},
	},
}

// toolRecommendations maps language → recommended external tools.
var toolRecommendations = map[string][]ToolRecommendation{
	"go": {
		{Category: "linter", ID: "golangci-lint", Name: "golangci-lint", Reason: "Go project detected"},
		{Category: "formatter", ID: "gofmt", Name: "gofmt", Reason: "Go project detected"},
		{Category: "formatter", ID: "goimports", Name: "goimports", Reason: "Go project detected"},
	},
	"javascript": {
		{Category: "linter", ID: "eslint", Name: "ESLint", Reason: "JavaScript project detected"},
		{Category: "formatter", ID: "prettier", Name: "Prettier", Reason: "JavaScript project detected"},
	},
	"typescript": {
		{Category: "linter", ID: "eslint", Name: "ESLint", Reason: "TypeScript project detected"},
		{Category: "formatter", ID: "prettier", Name: "Prettier", Reason: "TypeScript project detected"},
	},
	"python": {
		{Category: "linter", ID: "ruff", Name: "Ruff", Reason: "Python project detected"},
		{Category: "formatter", ID: "ruff-format", Name: "Ruff Format", Reason: "Python project detected"},
		{Category: "linter", ID: "pytest", Name: "pytest", Reason: "Python project detected"},
	},
	"rust": {
		{Category: "linter", ID: "clippy", Name: "Clippy", Reason: "Rust project detected"},
		{Category: "formatter", ID: "rustfmt", Name: "rustfmt", Reason: "Rust project detected"},
	},
	"java": {
		{Category: "linter", ID: "checkstyle", Name: "Checkstyle", Reason: "Java project detected"},
		{Category: "formatter", ID: "google-java-format", Name: "google-java-format", Reason: "Java project detected"},
	},
	"ruby": {
		{Category: "linter", ID: "rubocop", Name: "RuboCop", Reason: "Ruby project detected"},
	},
	"php": {
		{Category: "linter", ID: "phpstan", Name: "PHPStan", Reason: "PHP project detected"},
		{Category: "formatter", ID: "php-cs-fixer", Name: "PHP-CS-Fixer", Reason: "PHP project detected"},
	},
}

// coreModeRecommendations returns mode recommendations that apply to all detected languages.
func coreModeRecommendations(lang string) []ToolRecommendation {
	reason := lang + " project detected"
	return []ToolRecommendation{
		{Category: "mode", ID: "coder", Name: "Coder", Reason: reason},
		{Category: "mode", ID: "reviewer", Name: "Reviewer", Reason: reason},
		{Category: "mode", ID: "tester", Name: "Tester", Reason: reason},
		{Category: "mode", ID: "security", Name: "Security Auditor", Reason: reason},
		{Category: "mode", ID: "architect", Name: "Architect", Reason: reason},
	}
}

// corePipelineRecommendations returns pipeline recommendations that apply to all detected languages.
func corePipelineRecommendations(lang string) []ToolRecommendation {
	reason := lang + " project detected"
	return []ToolRecommendation{
		{Category: "pipeline", ID: "standard-dev", Name: "Standard Development", Reason: reason},
		{Category: "pipeline", ID: "review-only", Name: "Review Only", Reason: reason},
	}
}

// RecommendationsForLanguage returns all recommendations for a given language.
func RecommendationsForLanguage(lang string) []ToolRecommendation {
	var recs []ToolRecommendation
	recs = append(recs, coreModeRecommendations(lang)...)
	recs = append(recs, corePipelineRecommendations(lang)...)
	if tools, ok := toolRecommendations[lang]; ok {
		recs = append(recs, tools...)
	}
	return recs
}
