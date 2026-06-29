package memory

import "time"

const (
	TypeUser      = "user"
	TypeFeedback  = "feedback"
	TypeProject   = "project"
	TypeReference = "reference"

	IndexFileName = "MEMORY.md"
	MaxFileBytes  = 512 * 1024
)

var ValidTypes = map[string]struct{}{
	TypeUser:      {},
	TypeFeedback:  {},
	TypeProject:   {},
	TypeReference: {},
}

type Frontmatter struct {
	ID              string   `yaml:"id"`
	Title           string   `yaml:"title"`
	Type            string   `yaml:"type"`
	Description     string   `yaml:"description"`
	Tags            []string `yaml:"tags,omitempty"`
	CreatedAt       string   `yaml:"created_at,omitempty"`
	UpdatedAt       string   `yaml:"updated_at,omitempty"`
	SourceSessionID string   `yaml:"source_session_id,omitempty"`
	SourceTurnID    string   `yaml:"source_turn_id,omitempty"`
	Confidence      float64  `yaml:"confidence,omitempty"`
}

type Document struct {
	Frontmatter Frontmatter
	Body        string
}

type Record struct {
	ID                   string
	ProjectID            string
	RelativePath         string
	AbsolutePath         string
	Type                 string
	Title                string
	Description          string
	Tags                 []string
	ContentHash          string
	MTimeUnix            int64
	SizeBytes            int64
	TokenEstimate        int
	Enabled              bool
	DeletedAt            string
	CreatedAt            string
	UpdatedAt            string
	CreatedFromSessionID string
	CreatedFromTurnID    string
	LastIndexedAt        string
	LastInjectedAt       string
	Preview              string
	Content              string
}

type ScannedRecord struct {
	Record  Record
	Content string
}

type ScanIssue struct {
	RelativePath string
	Path         string
	Error        string
}

type ScanResult struct {
	Records []ScannedRecord
	Issues  []ScanIssue
}

type IndexResult struct {
	ProjectID string
	Indexed   int
	Deleted   int
	Failed    int
	Issues    []ScanIssue
	StartedAt string
	EndedAt   string
}

type SearchRequest struct {
	ProjectID   string
	Query       string
	Types       []string
	Tags        []string
	Limit       int
	TokenBudget int
}

type SearchResult struct {
	Record          Record
	Score           int
	SelectionReason string
	Content         string
}

type Injection struct {
	ID               string
	ProjectID        string
	SessionID        string
	TurnID           string
	MemoryID         string
	PromptAssemblyID string
	InjectedAt       string
	TokenEstimate    int
	SelectionReason  string
}

type Diagnostics struct {
	ProjectID   string
	MemoryRoot  string
	RecordCount int
	Enabled     int
	Disabled    int
	Deleted     int
	LastIndexed string
	Issues      []ScanIssue
}

func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
