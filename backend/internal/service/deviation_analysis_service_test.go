package service

import (
	"context"
	"encoding/json"
	"errors"
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

func TestTrendAggregatesSameVesselRecipeVersionAndChannel(t *testing.T) {
	db := newTestDB(t)
	vesselRepo := repository.NewFermentationVesselRepository(db)
	recipeRepo := repository.NewCultureRecipeRepository(db)
	seriesRepo := repository.NewSensorSeriesRepository(db)
	analysisRepo := repository.NewDeviationAnalysisRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	vessel := model.FermentationVessel{
		VesselCode: "FV-T1", Name: "Trend vessel", WorkingVolumeL: 100,
		SensorChannels: `["ph"]`, Location: "Lab", OwnerTeam: "Process",
		VesselState: "active", CommissionedAt: base, CreatedAt: base, UpdatedAt: base,
	}
	if err := vesselRepo.Create(context.Background(), &vessel); err != nil {
		t.Fatal(err)
	}
	boundaryJSON, err := json.Marshal([]algorithm.PhaseBoundary{
		{Phase: constants.PhaseLag, StartHour: 0, EndHour: 2},
		{Phase: constants.PhaseGrowth, StartHour: 2, EndHour: 4},
		{Phase: constants.PhaseProduction, StartHour: 4, EndHour: 6},
		{Phase: constants.PhaseHarvest, StartHour: 6, EndHour: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	curves := map[string][]algorithm.CurvePoint{"ph": {}, "temperature": {}}
	for hour := 0; hour <= 8; hour++ {
		curves["ph"] = append(curves["ph"], algorithm.CurvePoint{ElapsedHour: float64(hour), Value: 7 - float64(hour)*0.05})
		curves["temperature"] = append(curves["temperature"], algorithm.CurvePoint{ElapsedHour: float64(hour), Value: 29 + float64(hour)*0.1})
	}
	referenceJSON, err := json.Marshal(curves)
	if err != nil {
		t.Fatal(err)
	}
	toleranceJSON, err := json.Marshal(map[string]algorithm.ChannelTolerance{
		"ph": {Weight: 1, MaxDistance: 1}, "temperature": {Weight: 1, MaxDistance: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	recipe := model.CultureRecipe{
		VesselID: vessel.ID, RecipeCode: "TREND-A", Version: 1, Organism: "Test organism",
		TargetDurationH: 8, PhaseBoundariesJSON: string(boundaryJSON), ReferenceCurvesJSON: string(referenceJSON),
		ToleranceProfileJSON: string(toleranceJSON), RecipeState: "published",
		CreatedBy: 8, CreatedByName: "scientist", CreatedAt: base, UpdatedAt: base,
	}
	if err := recipeRepo.Create(context.Background(), &recipe); err != nil {
		t.Fatal(err)
	}
	otherRecipe := recipe
	otherRecipe.ID = 0
	otherRecipe.RecipeCode = "TREND-B"
	if err := recipeRepo.Create(context.Background(), &otherRecipe); err != nil {
		t.Fatal(err)
	}
	createReadySeries := func(runCode, channel string, start time.Time, slope float64, recipeID uint) model.SensorSeries {
		points := make([]timeseries.Point, 0, 9)
		for hour := 0; hour <= 8; hour++ {
			var value float64
			if channel == "temperature" {
				value = 29 + slope*float64(hour)
			} else {
				value = 7 - slope*float64(hour)
			}
			points = append(points, timeseries.Point{
				Timestamp: start.Add(time.Duration(hour) * time.Hour), Values: map[string]*float64{channel: &value},
			})
		}
		pointsJSON, err := timeseries.EncodePoints(points)
		if err != nil {
			t.Fatal(err)
		}
		record := model.SensorSeries{
			VesselID: vessel.ID, RecipeID: recipeID, RunCode: runCode, Channel: channel,
			SampleIntervalS: 3600, PointsJSON: pointsJSON, StartedAt: start, EndedAt: start.Add(8 * time.Hour),
			SourceChecksum: util.HashString(pointsJSON), SeriesState: "ready", QualitySummary: `{"valid":true}`,
			NormalizationJSON: `{"method":"median_iqr"}`, ImportedBy: 9, ImportedByName: "analyst",
			CreatedAt: start, UpdatedAt: start,
		}
		if err := seriesRepo.Create(context.Background(), &record); err != nil {
			t.Fatal(err)
		}
		return record
	}
	sameOne := createReadySeries("RUN-T1", "ph", base, 0.05, recipe.ID)
	sameTwo := createReadySeries("RUN-T2", "ph", base.Add(48*time.Hour), 0.18, recipe.ID)
	otherChannel := createReadySeries("RUN-T3", "temperature", base.Add(72*time.Hour), 0.05, recipe.ID)
	otherVersion := createReadySeries("RUN-T4", "ph", base.Add(96*time.Hour), 0.05, otherRecipe.ID)
	svc := NewDeviationAnalysisService(analysisRepo, recipeRepo, seriesRepo, auditRepo, algorithm.NewEvaluator())
	actor := util.Actor{UserID: 9, Username: "analyst", Role: "data_analyst", RequestID: "req-trend"}
	runAnalysis := func(seriesID uint, key string) dto.DeviationAnalysisResponse {
		result, _, err := svc.Run(context.Background(), dto.RunDeviationAnalysisRequest{SensorSeriesID: seriesID}, key, actor)
		if err != nil {
			t.Fatalf("run analysis %s: %v", key, err)
		}
		if result.OverallScore < 0 || result.OverallScore > 1 {
			t.Fatalf("overall score %.3f out of range", result.OverallScore)
		}
		return result
	}
	first := runAnalysis(sameOne.ID, "trend-key-1")
	second := runAnalysis(sameTwo.ID, "trend-key-2")
	runAnalysis(otherChannel.ID, "trend-key-3")
	runAnalysis(otherVersion.ID, "trend-key-4")
	trend, err := svc.Trend(context.Background(), second.ID)
	if err != nil {
		t.Fatalf("load trend: %v", err)
	}
	if !trend.Comparable || len(trend.Items) != 2 {
		t.Fatalf("trend comparable=%v items=%d, want 2 comparable batches", trend.Comparable, len(trend.Items))
	}
	if trend.Items[0].ID != first.ID || trend.Items[1].ID != second.ID {
		t.Fatalf("trend order = %d,%d; want %d,%d by analyzed time", trend.Items[0].ID, trend.Items[1].ID, first.ID, second.ID)
	}
	if trend.CurrentPoint == nil || trend.CurrentPoint.ID != second.ID {
		t.Fatalf("current point = %+v, want anchor %d", trend.CurrentPoint, second.ID)
	}
	if trend.VesselCode != "FV-T1" || trend.RecipeVersion != 1 || trend.Channel != "ph" {
		t.Fatalf("trend context = %+v", trend)
	}
	reviewer := util.Actor{UserID: 10, Username: "reviewer", Role: "reviewer", RequestID: "req-trend-review"}
	if _, err := svc.Transition(context.Background(), first.ID, dto.DeviationAnalysisTransitionRequest{ToState: "voided"}, reviewer); err != nil {
		t.Fatalf("void peer: %v", err)
	}
	afterVoid, err := svc.Trend(context.Background(), second.ID)
	if err != nil {
		t.Fatalf("reload trend: %v", err)
	}
	if afterVoid.Comparable || len(afterVoid.Items) != 1 {
		t.Fatalf("after void comparable=%v items=%d, want single remaining batch", afterVoid.Comparable, len(afterVoid.Items))
	}
}
