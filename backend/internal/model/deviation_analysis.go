package model
import (
	"encoding/json"
	"math"
	"time"
)
type DeviationAnalysis struct {
	ID                   uint         `gorm:"primaryKey" json:"id"`
	SensorSeriesID       uint         `gorm:"not null;index" json:"sensor_series_id"`
	SensorSeries         SensorSeries `gorm:"foreignKey:SensorSeriesID" json:"sensor_series,omitempty"`
	RecipeID             uint         `gorm:"not null;index" json:"recipe_id"`
	RecipeVersion        int          `gorm:"not null" json:"recipe_version"`
	AlgorithmVersion     string       `gorm:"size:40;not null;uniqueIndex:idx_analysis_input_algo" json:"algorithm_version"`
	InputHash            string       `gorm:"size:64;not null;uniqueIndex:idx_analysis_input_algo" json:"input_hash"`
	InputSnapshot        string       `gorm:"type:text;not null" json:"input_snapshot"`
	PhaseScoresJSON      string       `gorm:"type:text;not null" json:"phase_scores_json"`
	OverallScore         *float64     `gorm:"index" json:"overall_score,omitempty"`
	DeviationLevel       string       `gorm:"size:24;not null;index" json:"deviation_level"`
	AlignedCurveJSON     string       `gorm:"type:text;not null" json:"aligned_curve_json"`
	SuspectedCausesJSON  string       `gorm:"type:text;not null" json:"suspected_causes_json"`
	AnalysisState        string       `gorm:"size:24;not null;index" json:"analysis_state"`
	Explanation          string       `gorm:"type:text;not null" json:"explanation"`
	AnalyzedAt           time.Time    `gorm:"not null" json:"analyzed_at"`
	InitiatedBy          uint         `gorm:"not null;index" json:"initiated_by"`
	InitiatedByName      string       `gorm:"size:80;not null" json:"initiated_by_name"`
	ReviewedBy           *uint        `gorm:"index" json:"reviewed_by,omitempty"`
	ReviewedByName       string       `gorm:"size:80" json:"reviewed_by_name,omitempty"`
	IdempotencyKey       string       `gorm:"size:128;not null;uniqueIndex" json:"idempotency_key"`
	DurationMilliseconds int64        `gorm:"not null" json:"duration_milliseconds"`
	FailureReason        string       `gorm:"type:text" json:"failure_reason,omitempty"`
	ReviewComment        string       `gorm:"type:text" json:"review_comment,omitempty"`
	ReplayVerified       *bool        `json:"replay_verified,omitempty"`
	CreatedAt            time.Time    `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time    `gorm:"not null" json:"updated_at"`
}
func (DeviationAnalysis) TableName() string                    { return "deviation_analyses" }
func (a DeviationAnalysis) ReviewerSeparated(userID uint) bool { return a.InitiatedBy != userID }
// phaseScoreWeight mirrors algorithm.PhaseEvidence and is used to derive legacy overall scores.
type phaseScoreWeight struct {
	WeightedDeviation float64 `json:"weighted_deviation"`
}
// OverallDeviation returns the persisted multi-channel overall deviation. Rows created before the
// overall score was stored fall back to the equal-weight mean of per-phase weighted deviations.
func (a DeviationAnalysis) OverallDeviation() float64 {
	if a.OverallScore != nil {
		return *a.OverallScore
	}
	var scores []phaseScoreWeight
	if err := json.Unmarshal([]byte(a.PhaseScoresJSON), &scores); err != nil || len(scores) == 0 {
		return 0
	}
	total := 0.0
	for _, score := range scores {
		total += score.WeightedDeviation
	}
	return math.Round((total/float64(len(scores)))*1e6) / 1e6
}
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:80;not null;uniqueIndex" json:"username"`
	DisplayName  string    `gorm:"size:120;not null" json:"display_name"`
	PasswordHash string    `gorm:"size:100;not null" json:"-"`
	Role         string    `gorm:"size:40;not null;index" json:"role"`
	Active       bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}
func (User) TableName() string { return "users" }
type AuditLog struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	RequestID      string    `gorm:"size:80;not null;index" json:"request_id"`
	ActorID        uint      `gorm:"not null;index" json:"actor_id"`
	ActorName      string    `gorm:"size:80;not null" json:"actor_name"`
	ActorRole      string    `gorm:"size:40;not null" json:"actor_role"`
	EntityType     string    `gorm:"size:60;not null;index" json:"entity_type"`
	EntityID       uint      `gorm:"not null;index" json:"entity_id"`
	Action         string    `gorm:"size:80;not null;index" json:"action"`
	BeforeSnapshot string    `gorm:"type:text;not null" json:"before_snapshot"`
	AfterSnapshot  string    `gorm:"type:text;not null" json:"after_snapshot"`
	InputHash      string    `gorm:"size:64;index" json:"input_hash,omitempty"`
	Algorithm      string    `gorm:"size:40" json:"algorithm_version,omitempty"`
	DurationMS     int64     `json:"duration_ms,omitempty"`
	ResultSummary  string    `gorm:"type:text" json:"result_summary,omitempty"`
	CreatedAt      time.Time `gorm:"not null;index" json:"created_at"`
}
func (AuditLog) TableName() string { return "audit_logs" }
