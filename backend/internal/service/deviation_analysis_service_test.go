package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"fermentation-kinetics-deviation-analysis/backend/internal/algorithm"
	"fermentation-kinetics-deviation-analysis/backend/internal/constants"
	"fermentation-kinetics-deviation-analysis/backend/internal/dto"
	"fermentation-kinetics-deviation-analysis/backend/internal/model"
	"fermentation-kinetics-deviation-analysis/backend/internal/repository"
	"fermentation-kinetics-deviation-analysis/backend/internal/timeseries"
	"fermentation-kinetics-deviation-analysis/backend/internal/util"
)

func TestAnalysisIdempotencyReviewerSeparationAndReplay(t *testing.T) {
	db := newTestDB(t)
	vesselRepo := repository.NewFermentationVesselRepository(db)
	recipeRepo := repository.NewCultureRecipeRepository(db)
	seriesRepo := repository.NewSensorSeriesRepository(db)
	analysisRepo := repository.NewDeviationAnalysisRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	vessel := model.FermentationVessel{
		VesselCode: "FV-A1", Name: "Analysis vessel", WorkingVolumeL: 100,
		SensorChannels: `["ph"]`, Location: "Lab", OwnerTeam: "Process",
		VesselState: "active", CommissionedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := vesselRepo.Create(context.Background(), &vessel); err != nil {
		t.Fatal(err)
	}
	boundaries, references, tolerances := testRecipeConfig(t)
	recipe := model.CultureRecipe{
		VesselID: vessel.ID, RecipeCode: "ANALYSIS-A", Version: 1, Organism: "Test organism",
		TargetDurationH: 8, PhaseBoundariesJSON: string(boundaries), ReferenceCurvesJSON: string(references),
		ToleranceProfileJSON: string(tolerances), RecipeState: "published",
		CreatedBy: 8, CreatedByName: "scientist", CreatedAt: now, UpdatedAt: now,
	}
	if err := recipeRepo.Create(context.Background(), &recipe); err != nil {
		t.Fatal(err)
	}
	points := make([]timeseries.Point, 0, 9)
	for hour := 0; hour <= 8; hour++ {
		value := 7 - float64(hour)*0.05
		valueCopy := value
		points = append(points, timeseries.Point{
			Timestamp: now.Add(time.Duration(hour) * time.Hour), Values: map[string]*float64{"ph": &valueCopy},
		})
	}
	pointsJSON, err := timeseries.EncodePoints(points)
	if err != nil {
		t.Fatal(err)
	}
	series := model.SensorSeries{
		VesselID: vessel.ID, RecipeID: recipe.ID, RunCode: "RUN-A1", Channel: "ph",
		SampleIntervalS: 3600, PointsJSON: pointsJSON, StartedAt: now, EndedAt: now.Add(8 * time.Hour),
		SourceChecksum: util.HashString(pointsJSON), SeriesState: "ready", QualitySummary: `{"valid":true}`,
		NormalizationJSON: `{"method":"median_iqr"}`, ImportedBy: 9, ImportedByName: "analyst",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := seriesRepo.Create(context.Background(), &series); err != nil {
		t.Fatal(err)
	}
	svc := NewDeviationAnalysisService(analysisRepo, recipeRepo, seriesRepo, auditRepo, algorithm.NewEvaluator())
	initiator := util.Actor{UserID: 9, Username: "analyst", Role: "data_analyst", RequestID: "req-run"}
	first, reused, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: series.ID}, "idem-a", initiator)
	if err != nil || reused {
		t.Fatalf("first run reused=%v err=%v", reused, err)
	}
	second, reused, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: series.ID}, "idem-a", initiator)
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("same-key run id=%d reused=%v err=%v", second.ID, reused, err)
	}
	third, reused, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: series.ID}, "idem-b", initiator)
	if err != nil || !reused || third.ID != first.ID {
		t.Fatalf("same-input run id=%d reused=%v err=%v", third.ID, reused, err)
	}
	reviewer := util.Actor{UserID: 10, Username: "reviewer", Role: "reviewer", RequestID: "req-review"}
	if _, err := svc.Transition(context.Background(), first.ID, dto.DeviationAnalysisTransitionRequest{
		ToState: "reviewed", Comment: "Evidence reviewed.",
	}, reviewer); err != nil {
		t.Fatalf("review transition: %v", err)
	}
	_, err = svc.Transition(context.Background(), first.ID, dto.DeviationAnalysisTransitionRequest{
		ToState: "confirmed", Comment: "Self confirmation must fail.",
	}, initiator)
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != util.CodeReviewerConflict {
		t.Fatalf("self-confirm error=%v, want reviewer conflict", err)
	}
	if _, err := svc.Transition(context.Background(), first.ID, dto.DeviationAnalysisTransitionRequest{
		ToState: "confirmed", Comment: "Independent confirmation.",
	}, reviewer); err != nil {
		t.Fatalf("independent confirm: %v", err)
	}
	replayed, err := svc.Replay(context.Background(), first.ID, reviewer)
	if err != nil || replayed.ReplayVerified == nil || !*replayed.ReplayVerified {
		t.Fatalf("replay verified=%v err=%v", replayed.ReplayVerified, err)
	}
}

func TestRunRequiresReadySeriesAndIdempotencyKey(t *testing.T) {
	db := newTestDB(t)
	svc := NewDeviationAnalysisService(
		repository.NewDeviationAnalysisRepository(db), repository.NewCultureRecipeRepository(db),
		repository.NewSensorSeriesRepository(db), repository.NewAuditRepository(db), algorithm.NewEvaluator(),
	)
	_, _, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: 99}, "", util.Actor{})
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != util.CodeIdempotency {
		t.Fatalf("missing key error=%v", err)
	}
}

func TestAnalysisRoleContract(t *testing.T) {
	if constants.HasPermission(constants.RoleDataAnalyst, constants.PermissionAnalysisConfirm) {
		t.Fatal("data analyst should not receive confirm permission")
	}
}

func TestBatchTrendAggregatesSameVesselRecipeVersionAndChannel(t *testing.T) {
	db := newTestDB(t)
	vesselRepo := repository.NewFermentationVesselRepository(db)
	recipeRepo := repository.NewCultureRecipeRepository(db)
	seriesRepo := repository.NewSensorSeriesRepository(db)
	analysisRepo := repository.NewDeviationAnalysisRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	vessel := model.FermentationVessel{
		VesselCode: "FV-TREND", Name: "Trend vessel", WorkingVolumeL: 200,
		SensorChannels: `["ph"]`, Location: "Lab", OwnerTeam: "Process",
		VesselState: "active", CommissionedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := vesselRepo.Create(context.Background(), &vessel); err != nil {
		t.Fatal(err)
	}
	otherVessel := model.FermentationVessel{
		VesselCode: "FV-OTHER", Name: "Other vessel", WorkingVolumeL: 200,
		SensorChannels: `["ph"]`, Location: "Lab", OwnerTeam: "Process",
		VesselState: "active", CommissionedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := vesselRepo.Create(context.Background(), &otherVessel); err != nil {
		t.Fatal(err)
	}
	boundaries, references, tolerances := testRecipeConfig(t)
	recipe := model.CultureRecipe{
		VesselID: vessel.ID, RecipeCode: "TREND-A", Version: 1, Organism: "Test organism",
		TargetDurationH: 8, PhaseBoundariesJSON: string(boundaries), ReferenceCurvesJSON: string(references),
		ToleranceProfileJSON: string(tolerances), RecipeState: "published",
		CreatedBy: 8, CreatedByName: "scientist", CreatedAt: now, UpdatedAt: now,
	}
	if err := recipeRepo.Create(context.Background(), &recipe); err != nil {
		t.Fatal(err)
	}
	svc := NewDeviationAnalysisService(analysisRepo, recipeRepo, seriesRepo, auditRepo, algorithm.NewEvaluator())
	initiator := util.Actor{UserID: 9, Username: "analyst", Role: "data_analyst", RequestID: "req-run"}
	// Three comparable batches on the same vessel + recipe version + ph channel,
	// run on different days with growing deviation.
	anchors := make([]dto.DeviationAnalysisResponse, 0, 3)
	for i, deviation := range []float64{0.02, 0.25, 0.55} {
		seriesRecord := readyTrendSeries(t, seriesRepo, vessel.ID, recipe.ID, fmt.Sprintf("RUN-TREND-%d", i),
			"ph", now.AddDate(0, 0, i), deviation)
		result, _, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: seriesRecord.ID},
			fmt.Sprintf("idem-trend-%d", i), initiator)
		if err != nil {
			t.Fatalf("run comparable batch %d: %v", i, err)
		}
		anchors = append(anchors, result)
	}
	// Voided result on the same cohort must be excluded from the trend.
	voidedSeries := readyTrendSeries(t, seriesRepo, vessel.ID, recipe.ID, "RUN-TREND-VOID",
		"ph", now.AddDate(0, 0, 3), 0.7)
	voided, _, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: voidedSeries.ID},
		"idem-trend-void", initiator)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := util.Actor{UserID: 10, Username: "reviewer", Role: "reviewer", RequestID: "req-review"}
	if _, err := svc.Transition(context.Background(), voided.ID,
		dto.DeviationAnalysisTransitionRequest{ToState: "voided"}, reviewer); err != nil {
		t.Fatalf("void analysis: %v", err)
	}
	// Different channel on the same vessel/recipe must not join the cohort.
	temperatureSeries := readyTrendSeries(t, seriesRepo, vessel.ID, recipe.ID, "RUN-TREND-TEMP",
		"temperature", now.AddDate(0, 0, 4), 0.4)
	insertCompletedTrendAnalysis(t, analysisRepo, temperatureSeries.ID, recipe.ID, now.AddDate(0, 0, 4), 0.4)
	// Same channel on a different vessel must not join the cohort either.
	otherSeries := readyTrendSeries(t, seriesRepo, otherVessel.ID, recipe.ID, "RUN-TREND-OTHER",
		"ph", now.AddDate(0, 0, 5), 0.4)
	insertCompletedTrendAnalysis(t, analysisRepo, otherSeries.ID, recipe.ID, now.AddDate(0, 0, 5), 0.4)
	trend, err := svc.Trend(context.Background(), anchors[2].ID)
	if err != nil {
		t.Fatalf("load trend: %v", err)
	}
	if !trend.Comparable || len(trend.Points) != 3 {
		t.Fatalf("trend comparable=%v points=%d, want 3 comparable points", trend.Comparable, len(trend.Points))
	}
	if trend.Context.VesselCode != "FV-TREND" || trend.Context.Channel != "ph" || trend.Context.RecipeVersion != 1 {
		t.Fatalf("unexpected trend context: %+v", trend.Context)
	}
	for i := 1; i < len(trend.Points); i++ {
		if trend.Points[i].AnalyzedAt.Before(trend.Points[i-1].AnalyzedAt) {
			t.Fatal("trend points must be ordered chronologically")
		}
		if trend.Points[i].OverallDeviation <= trend.Points[i-1].OverallDeviation {
			t.Fatalf("seeded deviation should increase across batches: %+v", trend.Points)
		}
	}
	if len(trend.Points[0].PhaseDeviations) != 4 {
		t.Fatalf("each point must carry four phase deviations, got %d", len(trend.Points[0].PhaseDeviations))
	}
}

func TestBatchTrendSingleBatchIsNotComparable(t *testing.T) {
	db := newTestDB(t)
	vesselRepo := repository.NewFermentationVesselRepository(db)
	recipeRepo := repository.NewCultureRecipeRepository(db)
	seriesRepo := repository.NewSensorSeriesRepository(db)
	analysisRepo := repository.NewDeviationAnalysisRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	vessel := model.FermentationVessel{
		VesselCode: "FV-SOLO", Name: "Solo vessel", WorkingVolumeL: 100,
		SensorChannels: `["ph"]`, Location: "Lab", OwnerTeam: "Process",
		VesselState: "active", CommissionedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := vesselRepo.Create(context.Background(), &vessel); err != nil {
		t.Fatal(err)
	}
	boundaries, references, tolerances := testRecipeConfig(t)
	recipe := model.CultureRecipe{
		VesselID: vessel.ID, RecipeCode: "SOLO-A", Version: 1, Organism: "Test organism",
		TargetDurationH: 8, PhaseBoundariesJSON: string(boundaries), ReferenceCurvesJSON: string(references),
		ToleranceProfileJSON: string(tolerances), RecipeState: "published",
		CreatedBy: 8, CreatedByName: "scientist", CreatedAt: now, UpdatedAt: now,
	}
	if err := recipeRepo.Create(context.Background(), &recipe); err != nil {
		t.Fatal(err)
	}
	svc := NewDeviationAnalysisService(analysisRepo, recipeRepo, seriesRepo, auditRepo, algorithm.NewEvaluator())
	initiator := util.Actor{UserID: 9, Username: "analyst", Role: "data_analyst", RequestID: "req-run"}
	seriesRecord := readyTrendSeries(t, seriesRepo, vessel.ID, recipe.ID, "RUN-SOLO-1", "ph", now, 0.1)
	anchor, _, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: seriesRecord.ID},
		"idem-solo", initiator)
	if err != nil {
		t.Fatal(err)
	}
	trend, err := svc.Trend(context.Background(), anchor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if trend.Comparable || len(trend.Points) != 1 {
		t.Fatalf("single batch comparable=%v points=%d, want not comparable with one point", trend.Comparable, len(trend.Points))
	}
}

func readyTrendSeries(
	t *testing.T, seriesRepo repository.SensorSeriesRepository, vesselID, recipeID uint,
	runCode, channel string, started time.Time, deviation float64,
) model.SensorSeries {
	t.Helper()
	points := make([]timeseries.Point, 0, 9)
	for hour := 0; hour <= 8; hour++ {
		value := 7 - float64(hour)*0.05 - deviation*float64(hour)*0.1
		valueCopy := value
		points = append(points, timeseries.Point{
			Timestamp: started.Add(time.Duration(hour) * time.Hour), Values: map[string]*float64{channel: &valueCopy},
		})
	}
	pointsJSON, err := timeseries.EncodePoints(points)
	if err != nil {
		t.Fatal(err)
	}
	seriesRecord := model.SensorSeries{
		VesselID: vesselID, RecipeID: recipeID, RunCode: runCode, Channel: channel,
		SampleIntervalS: 3600, PointsJSON: pointsJSON, StartedAt: started, EndedAt: started.Add(8 * time.Hour),
		SourceChecksum: util.HashString(pointsJSON), SeriesState: "ready", QualitySummary: `{"valid":true}`,
		NormalizationJSON: `{"method":"median_iqr"}`, ImportedBy: 9, ImportedByName: "analyst",
		CreatedAt: started, UpdatedAt: started,
	}
	if err := seriesRepo.Create(context.Background(), &seriesRecord); err != nil {
		t.Fatal(err)
	}
	return seriesRecord
}

func insertCompletedTrendAnalysis(
	t *testing.T, analysisRepo repository.DeviationAnalysisRepository,
	seriesID, recipeID uint, analyzedAt time.Time, overall float64,
) {
	t.Helper()
	phaseJSON := fmt.Sprintf(
		`[{"phase":"lag","duration_deviation":0,"slope_deviation":0,"peak_time_deviation":0,"curve_distance":0,"weighted_deviation":%.2f,"channel_scores":{},"observed_points":3},`+
			`{"phase":"growth","duration_deviation":0,"slope_deviation":0,"peak_time_deviation":0,"curve_distance":0,"weighted_deviation":%.2f,"channel_scores":{},"observed_points":3},`+
			`{"phase":"production","duration_deviation":0,"slope_deviation":0,"peak_time_deviation":0,"curve_distance":0,"weighted_deviation":%.2f,"channel_scores":{},"observed_points":3},`+
			`{"phase":"harvest","duration_deviation":0,"slope_deviation":0,"peak_time_deviation":0,"curve_distance":0,"weighted_deviation":%.2f,"channel_scores":{},"observed_points":3}]`,
		overall, overall, overall, overall,
	)
	level := string(constants.DeviationLevelForScore(overall))
	record := model.DeviationAnalysis{
		SensorSeriesID: seriesID, RecipeID: recipeID, RecipeVersion: 1,
		AlgorithmVersion: algorithm.Version, InputHash: fmt.Sprintf("manual-hash-%d-%d", seriesID, analyzedAt.UnixNano()),
		InputSnapshot:       "{}",
		PhaseScoresJSON:     phaseJSON,
		OverallDeviation:    &overall,
		DeviationLevel:      level,
		AlignedCurveJSON:    "[]",
		SuspectedCausesJSON: "[]",
		AnalysisState:       string(constants.AnalysisCompleted),
		Explanation:         "manual cohort control",
		AnalyzedAt:          analyzedAt,
		InitiatedBy:         9, InitiatedByName: "analyst",
		IdempotencyKey: fmt.Sprintf("manual-key-%d-%d", seriesID, analyzedAt.UnixNano()),
		CreatedAt:      analyzedAt, UpdatedAt: analyzedAt,
	}
	if err := analysisRepo.Create(context.Background(), &record); err != nil {
		t.Fatal(err)
	}
}
