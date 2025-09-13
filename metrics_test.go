// Copyright 2025 Matthew Gall <me@matthewgall.dev>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsCollector(t *testing.T) {
	// Create test client and monitor
	client := NewOctopusClient("test-account", "test-key", false)
	monitor := NewSavingSessionMonitor(client, "test-account")
	
	// Create metrics collector
	collector := NewMetricsCollector(client, monitor)
	
	// Test metrics collection
	metrics := collector.collectMetrics()
	
	// Verify basic metrics are present
	expectedMetrics := []string{
		"octojoin_info",
		"octojoin_up",
		"octojoin_last_check_timestamp",
	}
	
	for _, metric := range expectedMetrics {
		if !strings.Contains(metrics, metric) {
			t.Errorf("Expected metric %s not found in output", metric)
		}
	}
	
	// Verify HELP and TYPE comments are present
	if !strings.Contains(metrics, "# HELP") {
		t.Error("Expected HELP comments in metrics output")
	}
	
	if !strings.Contains(metrics, "# TYPE") {
		t.Error("Expected TYPE comments in metrics output")
	}
}

func TestMetricsHTTPEndpoint(t *testing.T) {
	// Create test client and monitor
	client := NewOctopusClient("test-account", "test-key", false)
	monitor := NewSavingSessionMonitor(client, "test-account")
	
	// Create metrics collector
	collector := NewMetricsCollector(client, monitor)
	
	// Create test HTTP request
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	
	// Call ServeHTTP
	collector.ServeHTTP(w, req)
	
	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	expectedContentType := "text/plain; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("Expected Content-Type %s, got %s", expectedContentType, contentType)
	}
	
	// Check response body contains metrics
	body := w.Body.String()
	if !strings.Contains(body, "octojoin_info") {
		t.Error("Expected octojoin_info metric in response")
	}
	
	if !strings.Contains(body, "octojoin_up") {
		t.Error("Expected octojoin_up metric in response")
	}
}

func TestWriteMetric(t *testing.T) {
	client := NewOctopusClient("test-account", "test-key", false)
	monitor := NewSavingSessionMonitor(client, "test-account")
	collector := NewMetricsCollector(client, monitor)
	
	var sb strings.Builder
	
	// Test metric without labels
	collector.writeMetric(&sb, "test_metric", nil, 42.5)
	result := sb.String()
	expected := "test_metric 42.5\n"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
	
	// Test metric with labels
	sb.Reset()
	labels := map[string]string{
		"label1": "value1",
		"label2": "value2",
	}
	collector.writeMetric(&sb, "test_metric", labels, 100)
	result = sb.String()
	
	// Check that the result contains the metric name and labels
	if !strings.Contains(result, "test_metric{") {
		t.Error("Expected metric with labels")
	}
	if !strings.Contains(result, `label1="value1"`) {
		t.Error("Expected label1=value1 in output")
	}
	if !strings.Contains(result, `label2="value2"`) {
		t.Error("Expected label2=value2 in output")
	}
	if !strings.Contains(result, "100") {
		t.Error("Expected value 100 in output")
	}
}