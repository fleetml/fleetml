DELETE FROM drift_results WHERE length(metric) > 50;
ALTER TABLE drift_results ALTER COLUMN metric TYPE VARCHAR(50);
DROP TABLE IF EXISTS drift_points;
DROP TABLE IF EXISTS feature_histograms;
DROP TABLE IF EXISTS model_features;
