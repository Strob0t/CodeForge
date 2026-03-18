package service

import "testing"

func TestClassifyComplexity(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "short simple prompt",
			prompt: "fix typo in README",
			want:   "simple",
		},
		{
			name: "code block with multi-step refactoring",
			prompt: "Please refactor this function, then update the tests, and finally run the linter:\n" +
				"```go\nfunc Process(items []Item) error {\n\tfor _, item := range items {\n\t\tif err := validate(item); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn nil\n}\n```\n" +
				"1. Extract the validation logic into a separate middleware function\n" +
				"2. Add proper error handling with custom error types\n" +
				"3. Implement a factory pattern for the validators\n" +
				"4. Update the unit tests for the refactored code\n" +
				"5. Run the linter after that to check code quality\n" +
				"6. Make sure the API endpoint still works correctly",
			want: "complex",
		},
		{
			name: "reasoning with multi-step analysis and many dimensions",
			prompt: "Design a migration plan for moving our monolith to a microservice architecture.\n" +
				"1. First, analyze the trade-offs between the current monolith and the proposed microservices\n" +
				"2. Then compare the deployment complexity and database schema migration strategies\n" +
				"3. Evaluate the orchestration requirements and assess the implications for authentication\n" +
				"4. Finally, justify the trade-off between concurrency and simplicity\n" +
				"Consider the pros and cons of each approach across multiple files in the codebase.\n" +
				"Why would container orchestration with kubernetes be advantageous for our API gateway and middleware?",
			want: "reasoning",
		},
		{
			name: "medium with code-like output request",
			prompt: "Add a new handler function in the project to validate user input before saving. " +
				"The endpoint should return appropriate error messages for invalid fields. " +
				"Write the validation logic with proper error handling.",
			want: "medium",
		},
		{
			name:   "empty string returns simple",
			prompt: "",
			want:   "simple",
		},
		{
			name:   "single word returns simple",
			prompt: "hello",
			want:   "simple",
		},
		{
			name: "complex with file paths and technical terms",
			prompt: "Refactor the authentication middleware in internal/adapter/http/middleware.go to use JWT tokens. " +
				"Update the database schema and the API endpoint to handle OAuth authentication. " +
				"First, modify the middleware. Then update the handler. After that, write integration tests.",
			want: "complex",
		},
		{
			name: "reasoning with design analysis and multi-step plan",
			prompt: "Design a caching strategy for our API gateway and middleware layer.\n" +
				"Step 1: Analyze the trade-offs between Redis and in-memory caching for our architecture.\n" +
				"Step 2: Compare Redis with Memcached and assess which fits our orchestration architecture better.\n" +
				"Step 3: Evaluate the concurrency model for the caching proxy.\n" +
				"Step 4: Justify the deployment strategy considering serialization and endpoint latency.\n" +
				"Consider the advantages and disadvantages of each approach across the codebase.\n" +
				"Why would a distributed cache improve our database and middleware performance?",
			want: "reasoning",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyComplexity(tt.prompt)
			if got != tt.want {
				t.Errorf("ClassifyComplexity(%q) = %q, want %q", truncatePrompt(tt.prompt, 60), got, tt.want)
			}
		})
	}
}

// truncatePrompt shortens a string for test error output readability.
func truncatePrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
