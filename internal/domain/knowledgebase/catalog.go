package knowledgebase

// BuiltinCatalog contains the definitions for all built-in knowledge bases.
// These are seeded to the database at startup if not already present.
var BuiltinCatalog = []CreateRequest{
	{
		Name:        "go-stdlib",
		Description: "Go standard library patterns, idioms, and best practices",
		Category:    CategoryLanguage,
		Tags:        []string{"go", "golang", "stdlib", "idioms"},
		ContentPath: "data/knowledge/go-stdlib",
		Builtin:     true,
	},
	{
		Name:        "react-patterns",
		Description: "React component patterns, hooks, performance optimization, and best practices",
		Category:    CategoryFramework,
		Tags:        []string{"react", "javascript", "typescript", "hooks", "components"},
		ContentPath: "data/knowledge/react-patterns",
		Builtin:     true,
	},
	{
		Name:        "python-stdlib",
		Description: "Python standard library and language best practices",
		Category:    CategoryLanguage,
		Tags:        []string{"python", "stdlib", "idioms"},
		ContentPath: "data/knowledge/python-stdlib",
		Builtin:     true,
	},
	{
		Name:        "solid-principles",
		Description: "SOLID design principles with practical examples",
		Category:    CategoryParadigm,
		Tags:        []string{"solid", "oop", "design-principles"},
		ContentPath: "data/knowledge/solid-principles",
		Builtin:     true,
	},
	{
		Name:        "clean-architecture",
		Description: "Clean Architecture and Hexagonal Architecture patterns",
		Category:    CategoryParadigm,
		Tags:        []string{"clean-architecture", "hexagonal", "ports-adapters", "ddd"},
		ContentPath: "data/knowledge/clean-architecture",
		Builtin:     true,
	},
	{
		Name:        "ddd-patterns",
		Description: "Domain-Driven Design tactical and strategic patterns",
		Category:    CategoryParadigm,
		Tags:        []string{"ddd", "domain-driven-design", "aggregates", "bounded-contexts"},
		ContentPath: "data/knowledge/ddd-patterns",
		Builtin:     true,
	},
	{
		Name:        "security-owasp",
		Description: "OWASP Top 10 security guidelines and secure coding practices",
		Category:    CategorySecurity,
		Tags:        []string{"owasp", "security", "vulnerabilities", "secure-coding"},
		ContentPath: "data/knowledge/security-owasp",
		Builtin:     true,
	},
	{
		Name:        "rest-api-design",
		Description: "RESTful API design best practices, naming conventions, and patterns",
		Category:    CategoryFramework,
		Tags:        []string{"rest", "api", "http", "design"},
		ContentPath: "data/knowledge/rest-api-design",
		Builtin:     true,
	},
}
