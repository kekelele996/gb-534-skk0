package database
import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
	"fermentation-kinetics-deviation-analysis/backend/internal/algorithm"
	"fermentation-kinetics-deviation-analysis/backend/internal/config"
	"fermentation-kinetics-deviation-analysis/backend/internal/constants"
	"fermentation-kinetics-deviation-analysis/backend/internal/model"
	"fermentation-kinetics-deviation-analysis/backend/internal/timeseries"
	"fermentation-kinetics-deviation-analysis/backend/internal/util"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)
func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.DBDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DBDSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DBDriver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: false,
		TranslateError:                           true,
	})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.DBDriver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain database connection pool: %w", err)
	}
	if cfg.DBDriver == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	if err := seed(db); err != nil {
		return nil, err
	}
	return db, nil
}
func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.User{}, &model.FermentationVessel{}, &model.CultureRecipe{},
		&model.SensorSeries{}, &model.DeviationAnalysis{}, &model.AuditLog{},
	); err != nil {
		return fmt.Errorf("migrate database schema: %w", err)
	}
	return nil
}
type seedAccount struct {
	Username, DisplayName, Password string
	Role                            constants.Role
}
func seed(db *gorm.DB) error {
	accounts := []seedAccount{
		{Username: "admin", DisplayName: "System Administrator", Password: "admin123", Role: constants.RoleAdmin},
		{Username: "scientist", DisplayName: "Process Scientist", Password: "scientist123", Role: constants.RoleProcessScientist},
		{Username: "analyst", DisplayName: "Data Analyst", Password: "analyst123", Role: constants.RoleDataAnalyst},
		{Username: "reviewer", DisplayName: "Independent Reviewer", Password: "reviewer123", Role: constants.RoleReviewer},
		{Username: "auditor", DisplayName: "Quality Auditor", Password: "auditor123", Role: constants.RoleAuditor},
	}
	return db.Transaction(func(tx *gorm.DB) error {
		users := make(map[string]model.User, len(accounts))
		for _, account := range accounts {
			var user model.User
			err := tx.Where("username = ?", account.Username).First(&user).Error
			if err == nil {
				users[account.Username] = user
				continue
			}
			if !errorsIsNotFound(err) {
				return fmt.Errorf("find seed user %s: %w", account.Username, err)
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash seed password for %s: %w", account.Username, err)
			}
			now := time.Now().UTC()
			user = model.User{
				Username: account.Username, DisplayName: account.DisplayName, PasswordHash: string(hash),
				Role: string(account.Role), Active: true, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("create seed user %s: %w", account.Username, err)
			}
			users[account.Username] = user
		}
		return seedDomain(tx, users)
	})
}
func errorsIsNotFound(err error) bool { return err == gorm.ErrRecordNotFound }
// resyncSQLiteSequences advances sqlite_sequence to each table's MAX(id). The mattn SQLite
// driver does not update sqlite_sequence when a row with an explicit primary key is inserted,
// so subsequent auto-increment rows could collide with seeded identifiers.
func resyncSQLiteSequences(tx *gorm.DB, tables ...string) error {
	if tx.Dialector.Name() != "sqlite" {
		return nil
	}
	for _, table := range tables {
		var maxID uint
		if err := tx.Table(table).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
			return fmt.Errorf("read max id for %s: %w", table, err)
		}
		if err := tx.Exec("DELETE FROM sqlite_sequence WHERE name = ?", table).Error; err != nil {
			return fmt.Errorf("reset sqlite sequence for %s: %w", table, err)
		}
		if maxID > 0 {
			if err := tx.Exec("INSERT INTO sqlite_sequence(name, seq) VALUES(?, ?)", table, maxID).Error; err != nil {
				return fmt.Errorf("advance sqlite sequence for %s: %w", table, err)
			}
		}
	}
	return nil
}
func seedDomain(tx *gorm.DB, users map[string]model.User) error {
	var count int64
	if err := tx.Model(&model.FermentationVessel{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count seed fermentation vessels: %w", err)
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC().Truncate(time.Second)
	commissioned := now.AddDate(-2, 0, 0)
	vessels := []model.FermentationVessel{
		{
			VesselCode: "FV-201", Name: "North Pilot Fermenter", WorkingVolumeL: 1200,
			SensorChannels: `["ph","temperature","do","agitation"]`,
			Location:       "Pilot Hall North", OwnerTeam: "Upstream Development", VesselState: "active",
			CommissionedAt: commissioned, CreatedAt: now, UpdatedAt: now,
		},
		{
			VesselCode: "FV-305", Name: "Production Train Fermenter", WorkingVolumeL: 8500,
			SensorChannels: `["ph","temperature","do","agitation","offgas_co2"]`,
			Location:       "Manufacturing Suite 3", OwnerTeam: "Bioprocess Operations", VesselState: "active",
			CommissionedAt: commissioned.AddDate(-1, 0, 0), CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := tx.Create(&vessels).Error; err != nil {
		return fmt.Errorf("create seed fermentation vessels: %w", err)
	}
	boundaries, references, tolerances, err := seedRecipeConfiguration()
	if err != nil {
		return err
	}
	scientist := users["scientist"]
	recipes := []model.CultureRecipe{
		{
			VesselID: vessels[0].ID, RecipeCode: "YEAST-FEDBATCH-A", Version: 1,
			Organism: "Pichia pastoris", TargetDurationH: 24,
			PhaseBoundariesJSON: boundaries, ReferenceCurvesJSON: references, ToleranceProfileJSON: tolerances,
			RecipeState: string(constants.RecipePublished), CreatedBy: scientist.ID, CreatedByName: scientist.Username,
			CreatedAt: now.Add(-14 * 24 * time.Hour), UpdatedAt: now.Add(-12 * 24 * time.Hour),
		},
		{
			VesselID: vessels[1].ID, RecipeCode: "YEAST-FEDBATCH-B", Version: 1,
			Organism: "Saccharomyces cerevisiae", TargetDurationH: 24,
			PhaseBoundariesJSON: boundaries, ReferenceCurvesJSON: references, ToleranceProfileJSON: tolerances,
			RecipeState: string(constants.RecipeDraft), CreatedBy: scientist.ID, CreatedByName: scientist.Username,
			CreatedAt: now.Add(-3 * 24 * time.Hour), UpdatedAt: now.Add(-3 * 24 * time.Hour),
		},
	}
	if err := tx.Create(&recipes).Error; err != nil {
		return fmt.Errorf("create seed culture recipes: %w", err)
	}
	if err := resyncSQLiteSequences(tx, "users", "fermentation_vessels", "culture_recipes"); err != nil {
		return err
	}
	started := now.Add(-30 * time.Hour).Truncate(time.Hour)
	qualityJSON, err := json.Marshal(timeseries.QualitySummary{
		OriginalPointCount: 13, UniquePointCount: 13, DuplicateCount: 0,
		LongGapCount: 0, MaxGapSeconds: 7200,
		MissingRate: map[string]float64{"agitation": 0, "do": 0, "ph": 0, "temperature": 0},
		Channels:    []string{"agitation", "do", "ph", "temperature"}, Warnings: []string{}, Valid: true,
	})
	if err != nil {
		return fmt.Errorf("encode seed quality summary: %w", err)
	}
	type seedBatch struct {
		runCode     string
		startedAt   time.Time
		deviation   float64
		analyzedAt  time.Time
		idempotency string
	}
	// Same vessel, recipe version and channel, with rising deviation over time for trend review.
	batches := []seedBatch{
		{runCode: "RUN-2026-0815-A", startedAt: started.Add(-96 * time.Hour), deviation: 0.10, analyzedAt: now.Add(-90 * time.Hour), idempotency: "seed-analysis-001"},
		{runCode: "RUN-2026-0819-A", startedAt: started, deviation: 0.18, analyzedAt: now.Add(-2 * time.Hour), idempotency: "seed-analysis-002"},
		{runCode: "RUN-2026-0821-A", startedAt: now.Add(-28 * time.Hour).Truncate(time.Hour), deviation: 0.32, analyzedAt: now.Add(-40 * time.Minute), idempotency: "seed-analysis-003"},
	}
	analyst := users["analyst"]
	batchSeries := make([]model.SensorSeries, 0, len(batches))
	for _, batch := range batches {
		batchPoints := seedPoints(batch.startedAt, batch.deviation)
		batchPointsJSON, encodeErr := timeseries.EncodePoints(batchPoints)
		if encodeErr != nil {
			return fmt.Errorf("encode seed points for %s: %w", batch.runCode, encodeErr)
		}
		_, normalization, normalizeErr := timeseries.Normalize(batchPoints)
		if normalizeErr != nil {
			return fmt.Errorf("normalize seed points for %s: %w", batch.runCode, normalizeErr)
		}
		normalizationJSON, encodeErr := timeseries.EncodeNormalization(normalization)
		if encodeErr != nil {
			return fmt.Errorf("encode normalization for %s: %w", batch.runCode, encodeErr)
		}
		seedSeries := model.SensorSeries{
			VesselID: vessels[0].ID, RecipeID: recipes[0].ID, RunCode: batch.runCode,
			Channel: "multichannel", SampleIntervalS: 7200, PointsJSON: batchPointsJSON,
			StartedAt: batchPoints[0].Timestamp, EndedAt: batchPoints[len(batchPoints)-1].Timestamp,
			SourceChecksum: util.HashString(batchPointsJSON), SeriesState: string(constants.SeriesReady),
			QualitySummary: string(qualityJSON), NormalizationJSON: normalizationJSON,
			ImportedBy: analyst.ID, ImportedByName: analyst.Username,
			CreatedAt: batch.startedAt, UpdatedAt: batch.startedAt.Add(25 * time.Hour),
		}
		// Create rows individually so GORM backfills the IDs referenced by the frozen analyses.
		if err := tx.Create(&seedSeries).Error; err != nil {
			return fmt.Errorf("create seed sensor series %s: %w", batch.runCode, err)
		}
		batchSeries = append(batchSeries, seedSeries)
	}
	secondPoints := seedPoints(started.Add(26*time.Hour), 0.10)
	secondPointsJSON, err := timeseries.EncodePoints(secondPoints)
	if err != nil {
		return fmt.Errorf("encode second seed points: %w", err)
	}
	importedSeries := model.SensorSeries{
		VesselID: vessels[0].ID, RecipeID: recipes[0].ID, RunCode: "RUN-2026-0820-B",
		Channel: "multichannel", SampleIntervalS: 7200, PointsJSON: secondPointsJSON,
		StartedAt: secondPoints[0].Timestamp, EndedAt: secondPoints[len(secondPoints)-1].Timestamp,
		SourceChecksum: util.HashString(secondPointsJSON), SeriesState: string(constants.SeriesImported),
		QualitySummary: string(qualityJSON), NormalizationJSON: "{}",
		ImportedBy: analyst.ID, ImportedByName: analyst.Username, CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
	}
	if err := tx.Create(&importedSeries).Error; err != nil {
		return fmt.Errorf("create imported seed sensor series: %w", err)
	}
	for index, batch := range batches {
		seedSeries := batchSeries[index]
		snapshot := algorithm.NewSnapshot(seedSeries, recipes[0])
		result, evalErr := algorithm.NewEvaluator().Evaluate(snapshot)
		if evalErr != nil {
			return fmt.Errorf("evaluate seed deviation analysis for %s: %w", batch.runCode, evalErr)
		}
		inputHash, hashErr := snapshot.Hash()
		if hashErr != nil {
			return fmt.Errorf("hash seed analysis input for %s: %w", batch.runCode, hashErr)
		}
		snapshotJSON, canonicalErr := snapshot.Canonical()
		if canonicalErr != nil {
			return fmt.Errorf("freeze seed analysis input for %s: %w", batch.runCode, canonicalErr)
		}
		overallScore := result.OverallScore
		seedAnalysis := model.DeviationAnalysis{
			SensorSeriesID: seedSeries.ID, RecipeID: recipes[0].ID, RecipeVersion: recipes[0].Version,
			AlgorithmVersion: algorithm.Version, InputHash: inputHash, InputSnapshot: snapshotJSON,
			PhaseScoresJSON: result.PhaseScoresJSON, OverallScore: &overallScore,
			DeviationLevel: string(result.DeviationLevel),
			AlignedCurveJSON: result.AlignedCurveJSON, SuspectedCausesJSON: result.SuspectedCausesJSON,
			AnalysisState: string(constants.AnalysisCompleted), Explanation: result.Explanation,
			AnalyzedAt: batch.analyzedAt, InitiatedBy: analyst.ID, InitiatedByName: analyst.Username,
			IdempotencyKey: batch.idempotency, DurationMilliseconds: 4,
			CreatedAt: batch.analyzedAt, UpdatedAt: batch.analyzedAt,
		}
		if err := tx.Create(&seedAnalysis).Error; err != nil {
			return fmt.Errorf("create seed deviation analysis for %s: %w", batch.runCode, err)
		}
	}
	return nil
}
func seedRecipeConfiguration() (string, string, string, error) {
	boundaries := []algorithm.PhaseBoundary{
		{Phase: constants.PhaseLag, StartHour: 0, EndHour: 4},
		{Phase: constants.PhaseGrowth, StartHour: 4, EndHour: 10},
		{Phase: constants.PhaseProduction, StartHour: 10, EndHour: 20},
		{Phase: constants.PhaseHarvest, StartHour: 20, EndHour: 24},
	}
	references := map[string][]algorithm.CurvePoint{
		"ph": {}, "temperature": {}, "do": {}, "agitation": {},
	}
	for hour := 0.0; hour <= 24; hour += 2 {
		references["ph"] = append(references["ph"], algorithm.CurvePoint{ElapsedHour: hour, Value: 6.8 - 0.025*hour + 0.08*math.Sin(hour/4)})
		references["temperature"] = append(references["temperature"], algorithm.CurvePoint{ElapsedHour: hour, Value: 29.5 + 0.04*hour})
		references["do"] = append(references["do"], algorithm.CurvePoint{ElapsedHour: hour, Value: 68 - 1.45*hour + 5*math.Sin(hour/3)})
		references["agitation"] = append(references["agitation"], algorithm.CurvePoint{ElapsedHour: hour, Value: 320 + 9.5*hour})
	}
	tolerances := map[string]algorithm.ChannelTolerance{
		"ph":          {Weight: 1.3, MaxDistance: 0.8},
		"temperature": {Weight: 1.0, MaxDistance: 0.8},
		"do":          {Weight: 1.4, MaxDistance: 1.0},
		"agitation":   {Weight: 0.7, MaxDistance: 1.1},
	}
	boundaryJSON, err := util.CanonicalJSON(boundaries)
	if err != nil {
		return "", "", "", fmt.Errorf("encode seed phase boundaries: %w", err)
	}
	referenceJSON, err := util.CanonicalJSON(references)
	if err != nil {
		return "", "", "", fmt.Errorf("encode seed reference curves: %w", err)
	}
	toleranceJSON, err := util.CanonicalJSON(tolerances)
	if err != nil {
		return "", "", "", fmt.Errorf("encode seed tolerance profile: %w", err)
	}
	return boundaryJSON, referenceJSON, toleranceJSON, nil
}
func seedPoints(started time.Time, deviation float64) []timeseries.Point {
	points := make([]timeseries.Point, 0, 13)
	for hour := 0.0; hour <= 24; hour += 2 {
		ph := 6.8 - 0.025*hour + 0.08*math.Sin(hour/4) - deviation*math.Sin(hour/5)
		temperature := 29.5 + 0.04*hour + deviation*0.3*math.Sin(hour/3)
		oxygen := 68 - 1.45*hour + 5*math.Sin(hour/3) - deviation*7*math.Sin(hour/5)
		agitation := 320 + 9.5*hour + deviation*20*math.Sin(hour/4)
		points = append(points, timeseries.Point{
			Timestamp: started.Add(time.Duration(hour * float64(time.Hour))),
			Values: map[string]*float64{
				"ph": floatPointer(ph), "temperature": floatPointer(temperature),
				"do": floatPointer(oxygen), "agitation": floatPointer(agitation),
			},
		})
	}
	return points
}
func floatPointer(value float64) *float64 { return &value }
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("obtain database connection for close: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
