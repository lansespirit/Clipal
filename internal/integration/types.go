package integration

import "time"

type ProductID string

const (
	ProductClaudeCode ProductID = "claude"
	ProductCodexCLI   ProductID = "codex"
	ProductOpenCode   ProductID = "opencode"
	ProductGeminiCLI  ProductID = "gemini"
	ProductContinue   ProductID = "continue"
	ProductAider      ProductID = "aider"
	ProductGoose      ProductID = "goose"
)

type backupSnapshot struct {
	Product       ProductID `json:"product"`
	TargetPath    string    `json:"target_path"`
	TargetExisted bool      `json:"target_existed"`
	CreatedAt     time.Time `json:"created_at"`
	BackupDir     string    `json:"backup_dir"`
	Original      []byte    `json:"-"`
}

type State string

const (
	StateConfigured    State = "configured"
	StateNotConfigured State = "not_configured"
	StateError         State = "error"
)

type Status struct {
	Product         ProductID `json:"product"`
	State           State     `json:"state"`
	TargetPath      string    `json:"target_path"`
	BackupAvailable bool      `json:"backup_available"`
	Warning         string    `json:"warning,omitempty"`
}

type Result struct {
	Product ProductID `json:"product"`
	Status  Status    `json:"status"`
	Message string    `json:"message"`
}

type Preview struct {
	Product        ProductID `json:"product"`
	CurrentContent string    `json:"current_content"`
	PlannedContent string    `json:"planned_content"`
}

func SupportedProducts() []ProductID {
	return []ProductID{ProductClaudeCode, ProductCodexCLI, ProductOpenCode, ProductGeminiCLI, ProductContinue, ProductAider, ProductGoose}
}

func ProductName(product ProductID) string {
	switch product {
	case ProductClaudeCode:
		return "Claude Code"
	case ProductCodexCLI:
		return "Codex CLI"
	case ProductOpenCode:
		return "OpenCode"
	case ProductGeminiCLI:
		return "Gemini CLI"
	case ProductContinue:
		return "Continue"
	case ProductAider:
		return "Aider"
	case ProductGoose:
		return "Goose"
	default:
		return string(product)
	}
}
