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
type DeviationAnalysisResponse struct {
	ID                   uint                  `json:"id"`
	SensorSeriesID       uint                  `json:"sensor_series_id"`
	RecipeID             uint                  `json:"recipe_id"`
	RecipeVersion        int                   `json:"recipe_version"`
	AlgorithmVersion     string                `json:"algorithm_version"`
	InputHash            string                `json:"input_hash"`
	PhaseScoresJSON      json.RawMessage       `json:"phase_scores_json"`
	OverallScore         float64               `json:"overall_score"`
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
// DeviationTrendPoint is one comparable batch on the batch-to-batch trend view.
type DeviationTrendPoint struct {
	ID              uint            `json:"id"`
	SensorSeriesID  uint            `json:"sensor_series_id"`
	RunCode         string          `json:"run_code"`
	VesselID        uint            `json:"vessel_id"`
	VesselCode      string          `json:"vessel_code"`
	RecipeID        uint            `json:"recipe_id"`
	RecipeCode      string          `json:"recipe_code"`
	RecipeVersion   int             `json:"recipe_version"`
	Channel         string          `json:"channel"`
	OverallScore    float64         `json:"overall_score"`
	DeviationLevel  string          `json:"deviation_level"`
	AnalysisState   string          `json:"analysis_state"`
	PhaseScoresJSON json.RawMessage `json:"phase_scores_json"`
	AnalyzedAt      time.Time       `json:"analyzed_at"`
}
// DeviationTrendResponse aggregates result-bearing, non-voided analyses that share the anchor
// analysis vessel, recipe version and sensor channel, ordered by analysis time.
type DeviationTrendResponse struct {
	AnchorID       uint                  `json:"anchor_id"`
	VesselID       uint                  `json:"vessel_id"`
	VesselCode     string                `json:"vessel_code"`
	RecipeID       uint                  `json:"recipe_id"`
	RecipeCode     string                `json:"recipe_code"`
	RecipeVersion  int                   `json:"recipe_version"`
	Channel        string                `json:"channel"`
	Comparable     bool                  `json:"comparable"`
	Items          []DeviationTrendPoint `json:"items"`
	CurrentPoint   *DeviationTrendPoint  `json:"current_point,omitempty"`
}
func NewDeviationTrendResponse(anchor model.DeviationAnalysis, peers []model.DeviationAnalysis) DeviationTrendResponse {
	response := DeviationTrendResponse{
		AnchorID: anchor.ID, VesselID: anchor.SensorSeries.VesselID, VesselCode: anchor.SensorSeries.Vessel.VesselCode,
		RecipeID: anchor.RecipeID, RecipeCode: anchor.SensorSeries.Recipe.RecipeCode,
		RecipeVersion: anchor.RecipeVersion, Channel: anchor.SensorSeries.Channel,
		Comparable: len(peers) >= 2, Items: make([]DeviationTrendPoint, 0, len(peers)),
	}
	for _, peer := range peers {
		point := NewDeviationTrendPoint(peer)
		response.Items = append(response.Items, point)
		if peer.ID == anchor.ID {
			pointCopy := point
			response.CurrentPoint = &pointCopy
		}
	}
	return response
}
func NewDeviationTrendPoint(analysis model.DeviationAnalysis) DeviationTrendPoint {
	return DeviationTrendPoint{
		ID: analysis.ID, SensorSeriesID: analysis.SensorSeriesID, RunCode: analysis.SensorSeries.RunCode,
		VesselID: analysis.SensorSeries.VesselID, VesselCode: analysis.SensorSeries.Vessel.VesselCode,
		RecipeID: analysis.RecipeID, RecipeCode: analysis.SensorSeries.Recipe.RecipeCode,
		RecipeVersion: analysis.RecipeVersion, Channel: analysis.SensorSeries.Channel,
		OverallScore: analysis.OverallDeviation(), DeviationLevel: analysis.DeviationLevel,
		AnalysisState: analysis.AnalysisState, PhaseScoresJSON: rawJSON(analysis.PhaseScoresJSON),
		AnalyzedAt: analysis.AnalyzedAt,
	}
}
func NewDeviationAnalysisResponse(analysis model.DeviationAnalysis) DeviationAnalysisResponse {
	response := DeviationAnalysisResponse{
		ID: analysis.ID, SensorSeriesID: analysis.SensorSeriesID, RecipeID: analysis.RecipeID,
		RecipeVersion: analysis.RecipeVersion, AlgorithmVersion: analysis.AlgorithmVersion,
		InputHash: analysis.InputHash, PhaseScoresJSON: rawJSON(analysis.PhaseScoresJSON),
		OverallScore: analysis.OverallDeviation(),
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
