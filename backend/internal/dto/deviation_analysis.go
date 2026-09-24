package dto
import (
	"encoding/json"
	"fermentation-kinetics-deviation-analysis/backend/internal/model"
	"time"
)
type RunDeviationAnalysisRequest struct {
	SensorSeriesID uint `json:"sensor_series_id" binding:"required"`
}
type DeviationAnalysisTransitionRequest struct {
	ToState string `json:"to_state" binding:"required,oneof=reviewed confirmed investigating voided"`
	Comment string `json:"comment" binding:"omitempty,max=1000"`
}
type DeviationAnalysisQuery struct {
	SensorSeriesID, RecipeID uint
	State, Level, Initiator  string
	Page, PageSize           int
}
// PhaseScore mirrors the frozen per-phase evidence written by the DTW evaluator.
type PhaseScore struct {
	Phase             string             `json:"phase"`
	DurationDeviation float64            `json:"duration_deviation"`
	SlopeDeviation    float64            `json:"slope_deviation"`
	PeakTimeDeviation float64            `json:"peak_time_deviation"`
	CurveDistance     float64            `json:"curve_distance"`
	WeightedDeviation float64            `json:"weighted_deviation"`
	ChannelScores     map[string]float64 `json:"channel_scores"`
	ObservedPoints    int                `json:"observed_points"`
}
// DecodePhaseScores parses the frozen phase evidence JSON; an empty payload yields no scores.
func DecodePhaseScores(raw string) ([]PhaseScore, error) {
	if raw == "" {
		return []PhaseScore{}, nil
	}
	var scores []PhaseScore
	if err := json.Unmarshal([]byte(raw), &scores); err != nil {
		return nil, err
	}
	return scores, nil
}
type DeviationAnalysisResponse struct {
	ID                   uint                  `json:"id"`
	SensorSeriesID       uint                  `json:"sensor_series_id"`
	RecipeID             uint                  `json:"recipe_id"`
	RecipeVersion        int                   `json:"recipe_version"`
	AlgorithmVersion     string                `json:"algorithm_version"`
	InputHash            string                `json:"input_hash"`
	PhaseScoresJSON      json.RawMessage       `json:"phase_scores_json"`
	OverallDeviation     *float64              `json:"overall_deviation,omitempty"`
	DeviationLevel       string                `json:"deviation_level"`
	AlignedCurveJSON     json.RawMessage       `json:"aligned_curve_json"`
	SuspectedCausesJSON  json.RawMessage       `json:"suspected_causes_json"`
	AnalysisState        string                `json:"analysis_state"`
	Explanation          string                `json:"explanation"`
	AnalyzedAt           time.Time             `json:"analyzed_at"`
	InitiatedBy          uint                  `json:"initiated_by"`
	InitiatedByName      string                `json:"initiated_by_name"`
	ReviewedBy           *uint                 `json:"reviewed_by,omitempty"`
	ReviewedByName       string                `json:"reviewed_by_name,omitempty"`
	DurationMilliseconds int64                 `json:"duration_milliseconds"`
	FailureReason        string                `json:"failure_reason,omitempty"`
	ReviewComment        string                `json:"review_comment,omitempty"`
	ReplayVerified       *bool                 `json:"replay_verified,omitempty"`
	SensorSeries         *SensorSeriesResponse `json:"sensor_series,omitempty"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}
type DeviationAnalysisListResponse struct {
	Items []DeviationAnalysisResponse `json:"items"`
	Total int64                       `json:"total"`
	Page  int                         `json:"page"`
	Size  int                         `json:"page_size"`
}
// BatchTrendContext identifies the vessel/recipe/channel cohort a trend point belongs to.
type BatchTrendContext struct {
	VesselID      uint   `json:"vessel_id"`
	VesselCode    string `json:"vessel_code"`
	RecipeID      uint   `json:"recipe_id"`
	RecipeCode    string `json:"recipe_code"`
	RecipeVersion int    `json:"recipe_version"`
	Channel       string `json:"channel"`
}
// BatchTrendPoint is one completed, non-voided analysis inside the comparable cohort.
type BatchTrendPoint struct {
	AnalysisID        uint               `json:"analysis_id"`
	SensorSeriesID    uint               `json:"sensor_series_id"`
	RunCode           string             `json:"run_code"`
	AnalyzedAt        time.Time          `json:"analyzed_at"`
	OverallDeviation  float64            `json:"overall_deviation"`
	DeviationLevel    string             `json:"deviation_level"`
	AnalysisState     string             `json:"analysis_state"`
	PhaseDeviations   map[string]float64 `json:"phase_deviations"`
}
type BatchTrendResponse struct {
	Context        BatchTrendContext `json:"context"`
	AnchorAnalysis uint              `json:"anchor_analysis_id"`
	Points         []BatchTrendPoint `json:"points"`
	Comparable     bool              `json:"comparable"`
}
func NewDeviationAnalysisResponse(analysis model.DeviationAnalysis) DeviationAnalysisResponse {
	response := DeviationAnalysisResponse{
		ID: analysis.ID, SensorSeriesID: analysis.SensorSeriesID, RecipeID: analysis.RecipeID,
		RecipeVersion: analysis.RecipeVersion, AlgorithmVersion: analysis.AlgorithmVersion,
		InputHash: analysis.InputHash, PhaseScoresJSON: rawJSON(analysis.PhaseScoresJSON),
		OverallDeviation: analysis.OverallDeviation,
		DeviationLevel: analysis.DeviationLevel, AlignedCurveJSON: rawJSON(analysis.AlignedCurveJSON),
		SuspectedCausesJSON: rawJSON(analysis.SuspectedCausesJSON), AnalysisState: analysis.AnalysisState,
		Explanation: analysis.Explanation, AnalyzedAt: analysis.AnalyzedAt,
		InitiatedBy: analysis.InitiatedBy, InitiatedByName: analysis.InitiatedByName,
		ReviewedBy: analysis.ReviewedBy, ReviewedByName: analysis.ReviewedByName,
		DurationMilliseconds: analysis.DurationMilliseconds, FailureReason: analysis.FailureReason,
		ReviewComment: analysis.ReviewComment, ReplayVerified: analysis.ReplayVerified,
		CreatedAt: analysis.CreatedAt, UpdatedAt: analysis.UpdatedAt,
	}
	if analysis.SensorSeries.ID != 0 {
		s := NewSensorSeriesResponse(analysis.SensorSeries)
		response.SensorSeries = &s
	}
	return response
}
