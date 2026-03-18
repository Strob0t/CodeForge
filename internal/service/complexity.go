package service

import (
	"regexp"
	"strings"
)

// Heuristic weights mirror the Python ComplexityAnalyzer defaults
// (workers/codeforge/routing/complexity.py lines 9-17).
var complexityWeights = [7]struct {
	name   string
	weight float64
	score  func(string) float64
}{
	{"code_presence", 0.20, scoreCodePresence},
	{"reasoning_markers", 0.20, scoreReasoningMarkers},
	{"technical_terms", 0.15, scoreTechnicalTerms},
	{"prompt_length", 0.10, scorePromptLength},
	{"multi_step", 0.15, scoreMultiStep},
	{"context_requirements", 0.10, scoreContextRequirements},
	{"output_complexity", 0.10, scoreOutputComplexity},
}

// taskTypeBoost maps inferred task types to score boosts, mirroring
// _DEFAULT_TASK_TYPE_BOOST from the Python analyzer.
var taskTypeBoost = map[string]float64{
	"chat":     0.0,
	"code":     0.10,
	"debug":    0.20,
	"qa":       0.15,
	"refactor": 0.20,
	"review":   0.25,
	"plan":     0.25,
}

// taskPattern pairs a task type label with its detection regex.
type taskPattern struct {
	taskType string
	pattern  *regexp.Regexp
}

// taskPatterns is evaluated in order; first match wins (same as Python).
var taskPatterns = []taskPattern{
	{"review", regexp.MustCompile(`(?i)\b(review|check|audit|find bugs|code quality|code review|inspect|lint)\b`)},
	{"debug", regexp.MustCompile(`(?i)\b(fix|debug|error|bug|broken|not working|crash|exception|traceback|stacktrace|failing)\b`)},
	{"refactor", regexp.MustCompile(`(?i)\b(refactor|clean up|simplify|reorganize|restructure|rename|extract|inline)\b`)},
	{"plan", regexp.MustCompile(`(?i)\b(plan|design|architect|strategy|roadmap|approach|proposal|blueprint)\b`)},
	{"qa", regexp.MustCompile(`(?i)\b(tests?|testing|coverage|assertion|unit tests?|integration tests?|e2e|spec)\b`)},
	{"code", regexp.MustCompile(`(?i)\b(implement|write code|generate|create a function|add a method|write a|build|develop|code|program|script)\b`)},
}

// inferTaskType returns the first matching task type or "chat" as default.
func inferTaskType(prompt string) string {
	for _, tp := range taskPatterns {
		if tp.pattern.MatchString(prompt) {
			return tp.taskType
		}
	}
	return "chat"
}

// ClassifyComplexity returns a complexity tier for a user prompt using
// 7 rule-based heuristics + task-type boost, ported from the Python
// ComplexityAnalyzer (workers/codeforge/routing/complexity.py).
// Tiers: "simple" (<0.25), "medium" (<0.50), "complex" (<0.75), "reasoning" (>=0.75).
func ClassifyComplexity(prompt string) string {
	var weighted float64
	for _, h := range complexityWeights {
		weighted += h.score(prompt) * h.weight
	}

	// Apply task-type boost (same as Python analyzer line 204).
	taskType := inferTaskType(prompt)
	if boost, ok := taskTypeBoost[taskType]; ok {
		weighted += boost
	}

	// Clamp to [0, 1]
	if weighted > 1.0 {
		weighted = 1.0
	}
	switch {
	case weighted >= 0.75:
		return "reasoning"
	case weighted >= 0.50:
		return "complex"
	case weighted >= 0.25:
		return "medium"
	default:
		return "simple"
	}
}

// --- Heuristic scoring functions (each returns 0.0-1.0) ---

var (
	reCodeBlocks     = regexp.MustCompile("(?s)```.*?```|`[^`]+`")
	reFileExtensions = regexp.MustCompile(`(?i)\b\w+\.(py|go|ts|tsx|js|jsx|java|rs|cpp|c|h|rb|php|swift|kt|scala|sql|yaml|yml|json|toml|sh|bash)\b`)
	reImportPatterns = regexp.MustCompile(`(?i)\b(import|from|require|include|using|package)\b`)
	reCodeKeywords   = regexp.MustCompile(`\b(function|class|def|const|let|var|struct|enum|interface|type|fn|pub|impl|async|await)\b`)
)

func scoreCodePresence(prompt string) float64 {
	score := 0.0
	if reCodeBlocks.MatchString(prompt) {
		score += 0.4
	}
	extCount := len(reFileExtensions.FindAllString(prompt, -1))
	score += min64(0.3, float64(extCount)*0.1)
	if reImportPatterns.MatchString(prompt) {
		score += 0.15
	}
	kwCount := len(reCodeKeywords.FindAllString(prompt, -1))
	score += min64(0.15, float64(kwCount)*0.05)
	return min64(1.0, score)
}

var reReasoning = regexp.MustCompile(`(?i)\b(analy[sz]e|compare|trade[- ]?off|design|evaluate|pros and cons|which is better|should [iI]|why|explain the difference|consider|weigh|assess|critique|justify|what are the advantages|disadvantages|implications)\b`)

func scoreReasoningMarkers(prompt string) float64 {
	matches := len(reReasoning.FindAllString(prompt, -1))
	switch matches {
	case 0:
		return 0.0
	case 1:
		return 0.4
	case 2:
		return 0.7
	default:
		return 1.0
	}
}

// technicalTerms mirrors _TECHNICAL_TERMS from the Python analyzer.
var technicalTerms = []string{
	"api", "database", "schema", "microservice", "kubernetes", "docker",
	"ci/cd", "algorithm", "complexity", "architecture", "refactor",
	"migration", "orm", "rest", "graphql", "websocket", "grpc",
	"protobuf", "redis", "postgres", "mongodb", "nginx", "terraform",
	"ansible", "pipeline", "deployment", "container", "orchestration",
	"authentication", "authorization", "oauth", "jwt", "ssl", "tls",
	"encryption", "hashing", "caching", "load balancer", "proxy",
	"middleware", "endpoint", "payload", "serialization", "deserialization",
	"concurrency", "parallelism", "async", "thread", "mutex", "semaphore",
	"dependency injection", "singleton", "factory", "observer", "strategy",
}

func scoreTechnicalTerms(prompt string) float64 {
	lower := strings.ToLower(prompt)
	count := 0
	for _, term := range technicalTerms {
		if strings.Contains(lower, term) {
			count++
		}
	}
	switch {
	case count == 0:
		return 0.0
	case count <= 2:
		return 0.3
	case count <= 5:
		return 0.6
	case count <= 10:
		return 0.8
	default:
		return 1.0
	}
}

func scorePromptLength(prompt string) float64 {
	// Approximate tokens as len/4 (same as Python analyzer).
	tokens := float64(len(prompt)) / 4.0
	switch {
	case tokens < 15:
		return 0.0
	case tokens < 100:
		return 0.2
	case tokens < 300:
		return 0.5
	case tokens < 750:
		return 0.8
	default:
		return 1.0
	}
}

var reMultiStep = regexp.MustCompile(`(?im)(\d+\.\s|\bstep\s+\d+\b|\bfirst\b.*\bthen\b|\bfinally\b|\b(?:first|second|third|next|after that|lastly|subsequently)\b|^\s*[-*]\s+)`)

func scoreMultiStep(prompt string) float64 {
	matches := len(reMultiStep.FindAllString(prompt, -1))
	switch {
	case matches == 0:
		return 0.0
	case matches <= 2:
		return 0.4
	case matches <= 5:
		return 0.7
	default:
		return 1.0
	}
}

var reContext = regexp.MustCompile(`(?i)(/[\w./-]+\.\w+|\.?/[\w./-]+|\bcodebase\b|\brepository\b|\brepo\b|\bproject\b|\bacross files\b|\bmultiple files\b|\bseveral files\b|\bdirectory\b|\bfolder\b)`)

func scoreContextRequirements(prompt string) float64 {
	matches := len(reContext.FindAllString(prompt, -1))
	switch {
	case matches == 0:
		return 0.0
	case matches <= 2:
		return 0.4
	case matches <= 5:
		return 0.7
	default:
		return 1.0
	}
}

var reOutput = regexp.MustCompile(`(?i)\b(generate|implement|write|create a|build|full implementation|complete code|write a|develop|produce|code for|scaffold|boilerplate)\b`)

func scoreOutputComplexity(prompt string) float64 {
	matches := len(reOutput.FindAllString(prompt, -1))
	switch {
	case matches == 0:
		return 0.0
	case matches == 1:
		return 0.4
	case matches <= 3:
		return 0.7
	default:
		return 1.0
	}
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
