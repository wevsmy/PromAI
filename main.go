package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"

	"PromAI/pkg/report"
)

type Config struct {
	PrometheusURL string       `yaml:"prometheus_url"`
	MetricTypes   []MetricType `yaml:"metric_types"`
}

type MetricType struct {
	Type    string         `yaml:"type"`    // 指标类型
	Metrics []MetricConfig `yaml:"metrics"` // 指标配置
}

type MetricConfig struct {
	Name          string            `yaml:"name"`           // 指标名称
	Description   string            `yaml:"description"`    // 指标描述
	Query         string            `yaml:"query"`          // 查询语句
	Threshold     float64           `yaml:"threshold"`      // 阈值
	Unit          string            `yaml:"unit"`           // 单位
	Labels        map[string]string `yaml:"labels"`         // key是原始label名，value是显示的别名
	ThresholdType string            `yaml:"threshold_type"` // 阈值比较方式: "greater", "less", "equal", "greater_equal", "less_equal"
}

// 加载配置文件
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // 读取配置文件
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config // 定义配置结构体
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	} // 解析配置文件
	// 从环境变量中获取 PrometheusURL
	if envPrometheusURL := os.Getenv("PROMETHEUS_URL"); envPrometheusURL != "" {
		log.Printf("使用环境变量中的 Prometheus URL: %s", envPrometheusURL)
		config.PrometheusURL = envPrometheusURL
	} else {
		log.Printf("使用配置文件中的 Prometheus URL: %s", config.PrometheusURL)
	}
	return &config, nil // 返回配置结构体
}

// 初始化Prometheus客户端
func initPrometheusClient(url string) (v1.API, error) {
	client, err := api.NewClient(api.Config{
		Address: url,
	}) // 创建Prometheus客户端
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	return v1.NewAPI(client), nil // 返回Prometheus API客户端
}

// 收集指标数据
func collectMetrics(client v1.API, config *Config) (*report.ReportData, error) { // 收集指标数据
	ctx := context.Background() // 创建上下文,用于控制Prometheus查询的生命周期和超时

	data := &report.ReportData{
		Timestamp:    time.Now(),                           // 记录报告生成时间
		MetricGroups: make(map[string]*report.MetricGroup), // 使用map便于通过类型名快速查找指标组
		ChartData:    make(map[string]template.JS),         // 使用map便于通过图表ID快速查找图表数据
	}

	for _, metricType := range config.MetricTypes { // 遍历每个指标类型
		group := &report.MetricGroup{ // 创建指标组
			Type:          metricType.Type,
			MetricsByName: make(map[string][]report.MetricData), // 指标组中的指标按名称分组
		}
		data.MetricGroups[metricType.Type] = group // 将指标组添加到报告数据中

		for _, metric := range metricType.Metrics { // 遍历每个指标
			result, _, err := client.Query(ctx, metric.Query, time.Now()) // 查询指标数据
			if err != nil {
				log.Printf("警告: 查询指标 %s 失败: %v", metric.Name, err)
				continue
			}
			log.Printf("指标 [%s] 查询结果: %+v", metric.Name, result)

			switch v := result.(type) { // 将查询结果转换为具体类型
			case model.Vector: // 处理向量类型的查询结果
				metrics := make([]report.MetricData, 0, len(v)) // 创建一个切片存储指标数据,预分配容量以提高性能
				for _, sample := range v {                      // 遍历每个样本数据
					log.Printf("指标 [%s] 原始数据: %+v, 值: %+v", metric.Name, sample.Metric, sample.Value)

					availableLabels := make(map[string]string)
					for labelName, labelValue := range sample.Metric {
						availableLabels[string(labelName)] = string(labelValue)
					}

					labels := make([]report.LabelData, 0, len(metric.Labels))
					for configLabel, configAlias := range metric.Labels {
						labelValue := "-" // 默认值
						if rawValue, exists := availableLabels[configLabel]; exists && rawValue != "" {
							labelValue = rawValue
						} else {
							log.Printf("警告: 指标 [%s] 标签 [%s] 缺失或为空", metric.Name, configLabel)
						}

						labels = append(labels, report.LabelData{
							Name:  configLabel,
							Alias: configAlias,
							Value: labelValue,
						})
					}

					if !validateLabels(labels) {
						log.Printf("警告: 指标 [%s] 标签数据不完整，跳过该条记录", metric.Name)
						continue
					}

					metricData := report.MetricData{
						Name:        metric.Name,
						Description: metric.Description,
						Value:       float64(sample.Value),
						Threshold:   metric.Threshold,
						Unit:        metric.Unit,
						Status:      getStatus(float64(sample.Value), metric.Threshold, metric.ThresholdType),
						StatusText:  report.GetStatusText(getStatus(float64(sample.Value), metric.Threshold, metric.ThresholdType)),
						Timestamp:   time.Now(),
						Labels:      labels,
					}

					if err := validateMetricData(metricData, metric.Labels); err != nil {
						log.Printf("警告: 指标 [%s] 数据验证失败: %v", metric.Name, err)
						continue
					}

					metrics = append(metrics, metricData)
				}
				group.MetricsByName[metric.Name] = metrics
			}
		}
	}
	return data, nil
}

// 验证指标数据的完整性
func validateMetricData(data report.MetricData, configLabels map[string]string) error {
	if len(data.Labels) != len(configLabels) {
		return fmt.Errorf("标签数量不匹配: 期望 %d, 实际 %d",
			len(configLabels), len(data.Labels))
	}

	labelMap := make(map[string]bool)
	for _, label := range data.Labels {
		if _, exists := configLabels[label.Name]; !exists {
			return fmt.Errorf("发现未配置的标签: %s", label.Name)
		}
		if label.Value == "" || label.Value == "-" {
			return fmt.Errorf("标签 %s 值为空", label.Name)
		}
		labelMap[label.Name] = true
	}

	return nil
}

// 获取状态
func getStatus(value, threshold float64, thresholdType string) (status string) {
	if thresholdType == "" {
		thresholdType = "greater" // 默认值
	}
	switch thresholdType {
	case "greater":
		if value > threshold {
			return "critical"
		} else if value >= threshold*0.8 {
			return "warning"
		}
	case "greater_equal":
		if value >= threshold {
			return "critical"
		} else if value >= threshold*0.8 {
			return "warning"
		}
	case "less":
		if value < threshold {
			return "normal"
		} else if value <= threshold*1.2 {
			return "warning"
		}
	case "less_equal":
		if value <= threshold {
			return "normal"
		} else if value <= threshold*1.2 {
			return "warning"
		}
	case "equal":
		if value == threshold {
			return "normal"
		} else if value > threshold {
			return "critical"
		}
		return "critical"
	}
	return "normal"
}

// 验证标签数据的完整性
func validateLabels(labels []report.LabelData) bool {
	for _, label := range labels {
		if label.Value == "" || label.Value == "-" {
			return false
		}
	}
	return true
}

// 新增的函数，用于处理加载配置和初始化客户端
func setup(configPath string) (v1.API, *Config, error) {
	config, err := loadConfig(configPath) // 使用传入的配置路径
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	client, err := initPrometheusClient(config.PrometheusURL)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing Prometheus client: %w", err)
	}

	return client, config, nil
}

// 程序入口
func main() {
	configPath := flag.String("config", "config/config.yaml", "Path to configuration file") // 定义配置文件路径
	port := flag.String("port", "8091", "Port to run the HTTP server on")                   // 定义端口号
	flag.Parse()                                                                            // 解析命令行参数

	// 读取配置文件和初始化Prometheus客户端
	client, config, err := setup(*configPath) // 使用命令行参数中的配置路径
	if err != nil {
		log.Fatalf("Error setting up: %v", err)
	}

	http.HandleFunc("/getreport", func(w http.ResponseWriter, r *http.Request) {
		// Start the worker to collect metrics and generate the report
		data, err := collectMetrics(client, config) // 收集指标数据
		if err != nil {
			http.Error(w, "Failed to collect metrics", http.StatusInternalServerError)
			log.Printf("Error collecting metrics: %v", err)
			return
		}

		// Generate report
		reportFilePath, err := report.GenerateReport(*data)
		if err != nil {
			http.Error(w, "Failed to generate report", http.StatusInternalServerError)
			log.Printf("Error generating report: %v", err)
			return
		}
		log.Println("Report generated successfully:", reportFilePath)

		// 在新的浏览器标签打开生成的报告
		http.Redirect(w, r, "/"+reportFilePath, http.StatusSeeOther)

	})
	// 提供静态文件服务以便访问生成的报告
	http.Handle("/reports/", http.StripPrefix("/reports/", http.FileServer(http.Dir("reports"))))

	// 启动 HTTP 服务器
	log.Printf("Starting server on port: %s with config: %s", *port, *configPath)
	log.Printf("Prometheus URL: %s", config.PrometheusURL)
	// 打印获取报告的地址
	log.Printf("获取报告地址: http://localhost:%s/getreport", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}
