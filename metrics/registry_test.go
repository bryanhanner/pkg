// Copyright (c) 2018 Palantir Technologies. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics_test

import (
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/pkg/metrics"
)

func TestRegistryRegistration(t *testing.T) {
	// register root metrics
	root := metrics.NewRootMetricsRegistry()

	// register metric
	_ = root.Counter("my-counter")
	// create subregistry and register metric on it
	sub := root.Subregistry("subregistry")
	_ = sub.Gauge("sub-gauge")

	wantNames := []string{
		"my-counter",
		"subregistry.sub-gauge",
	}

	var gotNames []string
	root.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		gotNames = append(gotNames, name)
		assert.NotNil(t, metric)
	}))
	assert.Equal(t, wantNames, gotNames)
}

func TestMetricsWithTags(t *testing.T) {
	root := metrics.NewRootMetricsRegistry()

	// register metric with tags
	_ = root.Counter("my-counter", metrics.MustNewTag("region", "nw"))
	_ = root.Counter("my-counter", metrics.MustNewTag("region", "ne"))
	_ = root.Counter("my-counter", metrics.MustNewTag("region", "se"), metrics.MustNewTag("application", "database"))

	var gotNames []string
	var gotTags [][]metrics.Tag

	root.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		gotNames = append(gotNames, name)
		gotTags = append(gotTags, tags)
		assert.NotNil(t, metric)
	}))

	// output is sorted by metric name and then by tag names (which themselves are sorted alphabetically)
	wantNames := []string{
		"my-counter",
		"my-counter",
		"my-counter",
	}
	wantTags := [][]metrics.Tag{
		{metrics.MustNewTag("application", "database"), metrics.MustNewTag("region", "se")},
		{metrics.MustNewTag("region", "ne")},
		{metrics.MustNewTag("region", "nw")},
	}
	assert.Equal(t, wantNames, gotNames)
	assert.Equal(t, wantTags, gotTags)
}

// Prefix should be used as provided (no case conversion/normalization), while tags should always be converted to
// lowercase.
func TestMetricsCasing(t *testing.T) {
	root := metrics.NewRootMetricsRegistry()

	// register metric with tags
	_ = root.Counter("my-COUNTER", metrics.MustNewTag("REGION", "nW"))
	_ = root.Counter("my-counter", metrics.MustNewTag("region", "NE"))

	var gotNames []string
	var gotTags [][]metrics.Tag

	root.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		gotNames = append(gotNames, name)
		gotTags = append(gotTags, tags)
		assert.NotNil(t, metric)
	}))

	// output is sorted by metric name and then by tag names (which themselves are sorted alphabetically)
	wantNames := []string{
		"my-COUNTER",
		"my-counter",
	}
	wantTags := [][]metrics.Tag{
		{metrics.MustNewTag("region", "nw")},
		{metrics.MustNewTag("region", "ne")},
	}
	assert.Equal(t, wantNames, gotNames)
	assert.Equal(t, wantTags, gotTags)
}

func TestRegistryRegistrationWithMemStats(t *testing.T) {
	// register root metrics
	root := metrics.NewRootMetricsRegistry()
	metrics.CaptureRuntimeMemStats(root, time.Hour)

	// register metric
	_ = root.Counter("my-counter")

	// create subregistry and register metric on it
	sub := root.Subregistry("subregistry")
	_ = sub.Gauge("sub-gauge")

	wantNames := []string{
		"go.runtime.MemStats.Alloc",
		"go.runtime.MemStats.Frees",
		"go.runtime.MemStats.GCCPUFraction",
		"go.runtime.MemStats.HeapAlloc",
		"go.runtime.MemStats.HeapIdle",
		"go.runtime.MemStats.HeapInuse",
		"go.runtime.MemStats.HeapObjects",
		"go.runtime.MemStats.HeapReleased",
		"go.runtime.MemStats.HeapSys",
		"go.runtime.MemStats.Mallocs",
		"go.runtime.MemStats.NumGC",
		"go.runtime.MemStats.PauseNs",
		"go.runtime.MemStats.PauseTotalNs",
		"go.runtime.MemStats.StackInuse",
		"go.runtime.MemStats.StackSys",
		"go.runtime.NumCgoCall",
		"go.runtime.NumGoroutine",
		"go.runtime.NumThread",
		"go.runtime.ReadMemStats",
		"my-counter",
		"subregistry.sub-gauge",
	}

	var gotNames []string
	root.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		gotNames = append(gotNames, name)
		assert.NotNil(t, metric)
	}))
	assert.Equal(t, wantNames, gotNames)
}

func concurrentMetricTest(t *testing.T) {
	root := metrics.NewRootMetricsRegistry()
	commonMetric := "test-counter"
	increments := 100

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		for i := 0; i < increments; i++ {
			root.Counter(commonMetric).Inc(1)
		}
		waitGroup.Done()
	}()
	go func() {
		for i := 0; i < increments; i++ {
			root.Counter(commonMetric).Inc(1)
		}
		waitGroup.Done()
	}()
	waitGroup.Wait()
	require.Equal(t, int64(2*increments), root.Counter(commonMetric).Count())
}

// It is hard to catch the goroutine exits and have them impact actual test reporting. We end up having
// to simulate the testing ourselves, but it also means that if this test fails, it takes a bit of work to figure out why.
func TestManyConcurrentMetrics(t *testing.T) {
	if os.Getenv("CRASH_IF_FAILS") == "1" {
		concurrentMetricTest(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestManyConcurrentMetrics")
	cmd.Env = append(os.Environ(), "CRASH_IF_FAILS=1")
	err := cmd.Run()
	require.NoError(t, err, "Error while checking for concurrent metric handling!")
}

func TestSubregistry_Each(t *testing.T) {
	rootRegistry := metrics.NewRootMetricsRegistry()
	subRegistry := rootRegistry.Subregistry("prefix.")
	subRegistry.Gauge("gauge1").Update(0)
	subRegistry.Gauge("gauge2").Update(1)
	gauge1Count := 0
	gauge2Count := 0
	subRegistry.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		assert.NotNil(t, metric)
		assert.Empty(t, tags)
		switch name {
		case "gauge1":
			gauge1Count++
		case "gauge2":
			gauge2Count++
		default:
			assert.Fail(t, "unexpected metric %s", name)
		}
	}))
	assert.Equal(t, 1, gauge1Count)
	assert.Equal(t, 1, gauge2Count)
}

func TestSubregistry_Unregister(t *testing.T) {
	registry := metrics.NewRootMetricsRegistry().Subregistry("prefix.")
	registry.Gauge("gauge1", metrics.MustNewTag("tagKey", "tagValue1")).Update(0)
	registry.Gauge("gauge1", metrics.MustNewTag("tagKey", "tagValue2")).Update(0)
	registry.Gauge("gauge2").Update(0)
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue1")}))
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue2")}))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 3, registrySize(t, registry))
	registry.Unregister("gauge1", metrics.MustNewTag("tagKey", "tagValue1"))
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue2")}))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 2, registrySize(t, registry))
	registry.Unregister("gauge1", metrics.MustNewTag("tagKey", "tagValue2"))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 1, registrySize(t, registry))
	registry.Unregister("gauge2")
	assert.Equal(t, 0, registrySize(t, registry))
}

func TestRootRegistry_Unregister(t *testing.T) {
	registry := metrics.NewRootMetricsRegistry()
	registry.Gauge("gauge1", metrics.MustNewTag("tagKey", "tagValue1")).Update(0)
	registry.Gauge("gauge1", metrics.MustNewTag("tagKey", "tagValue2")).Update(0)
	registry.Gauge("gauge2").Update(0)
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue1")}))
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue2")}))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 3, registrySize(t, registry))
	registry.Unregister("gauge1", metrics.MustNewTag("tagKey", "tagValue1"))
	assert.True(t, registryContains(t, registry, "gauge1", []metrics.Tag{metrics.MustNewTag("tagKey", "tagValue2")}))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 2, registrySize(t, registry))
	registry.Unregister("gauge1", metrics.MustNewTag("tagKey", "tagValue2"))
	assert.True(t, registryContains(t, registry, "gauge2", nil))
	assert.Equal(t, 1, registrySize(t, registry))
	registry.Unregister("gauge2")
	assert.Equal(t, 0, registrySize(t, registry))
}

func TestRootRegistry_SubregistryWithTags(t *testing.T) {
	rootRegistry := metrics.NewRootMetricsRegistry()

	permanentTag := metrics.MustNewTag("permanentKey", "permanentValue")
	subregistry := rootRegistry.Subregistry("subregistry", permanentTag)

	runtimeTag := metrics.MustNewTag("key", "value")
	subregistry.Counter("counter", runtimeTag).Count()
	subregistry.Gauge("gauge", runtimeTag).Update(0)
	subregistry.GaugeFloat64("gaugeFloat64", runtimeTag).Update(0)
	subregistry.Meter("meter", runtimeTag).Mark(0)
	subregistry.Timer("timer", runtimeTag).Update(0)
	subregistry.Histogram("histogram", runtimeTag).Update(0)
	subregistry.HistogramWithSample("histogramWithSample", metrics.DefaultSample(), runtimeTag).Update(0)

	registered := map[string]map[string]string{}
	subregistry.Each(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		registered[name] = tags.ToMap()
	})

	assert.Equal(t,
		map[string]map[string]string{
			"counter":             metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"gauge":               metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"gaugeFloat64":        metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"meter":               metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"timer":               metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"histogram":           metrics.Tags{permanentTag, runtimeTag}.ToMap(),
			"histogramWithSample": metrics.Tags{permanentTag, runtimeTag}.ToMap(),
		},
		registered,
	)

	subregistry.Unregister("counter", runtimeTag)
	subregistry.Unregister("gauge", runtimeTag)
	subregistry.Unregister("gaugeFloat64", runtimeTag)
	subregistry.Unregister("meter", runtimeTag)
	subregistry.Unregister("timer", runtimeTag)
	subregistry.Unregister("histogram", runtimeTag)
	subregistry.Unregister("histogramWithSample", runtimeTag)

	subregistry.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		assert.Fail(t, "there should be no metrics registered")
	}))
}

func registrySize(t *testing.T, registry metrics.Registry) int {
	count := 0
	registry.Each(metrics.MetricVisitor(func(name string, tags metrics.Tags, metric metrics.MetricVal) {
		count++
	}))
	return count
}

func registryContains(t *testing.T, registry metrics.Registry, name string, tags metrics.Tags) bool {
	contains := false
	tagStrings := []string{}
	for _, tag := range tags {
		tagStrings = append(tagStrings, tag.String())
	}
	registry.Each(metrics.MetricVisitor(func(eachName string, eachTags metrics.Tags, metric metrics.MetricVal) {
		eachTagStrings := []string{}
		for _, eachTag := range eachTags {
			eachTagStrings = append(eachTagStrings, eachTag.String())
		}
		if eachName == name && reflect.DeepEqual(eachTagStrings, tagStrings) {
			contains = true
		}
	}))
	return contains
}
