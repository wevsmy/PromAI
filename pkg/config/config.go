package config

type Config struct {
	PrometheusURL string       `yaml:"prometheus_url"`
	MetricTypes   []MetricType `yaml:"metric_types"`
	ProjectName   string       `yaml:"project_name"`
}

type MetricType struct {
	Type    string         `yaml:"type"`
	Metrics []MetricConfig `yaml:"metrics"`
}

type MetricConfig struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	Query         string            `yaml:"query"`
	Threshold     float64           `yaml:"threshold"`
	Unit          string            `yaml:"unit"`
	Labels        map[string]string `yaml:"labels"`
	ThresholdType string            `yaml:"threshold_type"`
}
