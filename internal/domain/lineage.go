package domain

import "time"

// LineageNode is a read-only projection of one project object. Key is stable
// across API, CLI, and Web consumers and is formed as "type:id".
type LineageNode struct {
	Key       string         `json:"key"`
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	Label     string         `json:"label"`
	Status    string         `json:"status"`
	Stage     string         `json:"stage"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type LineageEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
	Reason   string `json:"reason"`
}

type LineageGraph struct {
	ProjectID   string         `json:"project_id"`
	FocusKey    string         `json:"focus_key,omitempty"`
	Direction   string         `json:"direction"`
	Nodes       []LineageNode  `json:"nodes"`
	Edges       []LineageEdge  `json:"edges"`
	StageCount  map[string]int `json:"stage_count"`
	GeneratedAt time.Time      `json:"generated_at"`
}

type ImpactItem struct {
	Node              LineageNode `json:"node"`
	Depth             int         `json:"depth"`
	Severity          string      `json:"severity"`
	Reason            string      `json:"reason"`
	CurrentStatus     string      `json:"current_status"`
	RecommendedAction string      `json:"recommended_action"`
}

type ImpactAnalysis struct {
	ProjectID   string       `json:"project_id"`
	Focus       *LineageNode `json:"focus,omitempty"`
	Items       []ImpactItem `json:"items"`
	GeneratedAt time.Time    `json:"generated_at"`
}
