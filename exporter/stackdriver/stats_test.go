// Copyright 2017, OpenCensus Authors
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

package stackdriver

import (
	"context"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3"
	"github.com/golang/protobuf/ptypes/timestamp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/option"
	distributionpb "google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/label"
	"google.golang.org/genproto/googleapis/api/metric"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var authOptions = []option.ClientOption{option.WithGRPCConn(&grpc.ClientConn{})}

func TestRejectBlankProjectID(t *testing.T) {
	ids := []string{"", "     ", " "}
	for _, projectID := range ids {
		opts := Options{ProjectID: projectID, ClientOptions: authOptions}
		exp, err := newStatsExporter(opts)
		if err == nil || exp != nil {
			t.Errorf("%q ProjectID must be rejected: NewExporter() = %v err = %q", projectID, exp, err)
		}
	}
}

// Ensure only one exporter per projectID per process, any
// subsequent invocations of NewExporter should fail.
func TestNewExporterSingletonPerProcess(t *testing.T) {
	ids := []string{"open-census.io", "x", "fakeProjectID"}
	for _, projectID := range ids {
		opts := Options{ProjectID: projectID, ClientOptions: authOptions}
		exp, err := newStatsExporter(opts)
		if err != nil {
			t.Errorf("NewExporter() projectID = %q err = %q", projectID, err)
			continue
		}
		if exp == nil {
			t.Errorf("NewExporter returned a nil Exporter")
			continue
		}
		exp, err = newStatsExporter(opts)
		if err == nil || exp != nil {
			t.Errorf("NewExporter more than once should fail; exp (%v) err %v", exp, err)
		}
	}
}

func TestExporter_makeReq(t *testing.T) {
	m, err := stats.Float64("test-measure", "measure desc", "unit")
	if err != nil {
		t.Fatal(err)
	}

	key, err := tag.NewKey("test_key")
	if err != nil {
		t.Fatal(err)
	}

	v := &view.View{
		Name:        "testview",
		Description: "desc",
		TagKeys:     []tag.Key{key},
		Measure:     m,
		Aggregation: view.Count(),
	}
	distView := &view.View{
		Name:        "distview",
		Description: "desc",
		Measure:     m,
		Aggregation: view.Distribution(2, 4, 7),
	}

	start := time.Now()
	end := start.Add(time.Minute)
	count1 := view.CountData(10)
	count2 := view.CountData(16)
	sum1 := view.SumData(5.5)
	sum2 := view.SumData(-11.1)
	mean1 := view.MeanData{
		Mean:  3.3,
		Count: 7,
	}
	mean2 := view.MeanData{
		Mean:  -7.7,
		Count: 5,
	}
	taskValue := getTaskValue()

	tests := []struct {
		name   string
		projID string
		vd     *view.Data
		want   []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			name:   "count agg + timeline",
			projID: "proj-id",
			vd:     newTestViewData(v, start, end, &count1, &count2),
			want: []*monitoringpb.CreateTimeSeriesRequest{{
				Name: monitoring.MetricProjectPath("proj-id"),
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-1",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{
									Int64Value: 10,
								}},
							},
						},
					},
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-2",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{
									Int64Value: 16,
								}},
							},
						},
					},
				},
			}},
		},
		{
			name:   "sum agg + timeline",
			projID: "proj-id",
			vd:     newTestViewData(v, start, end, &sum1, &sum2),
			want: []*monitoringpb.CreateTimeSeriesRequest{{
				Name: monitoring.MetricProjectPath("proj-id"),
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-1",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{
									DoubleValue: 5.5,
								}},
							},
						},
					},
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-2",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{
									DoubleValue: -11.1,
								}},
							},
						},
					},
				},
			}},
		},
		{
			name:   "mean agg + timeline",
			projID: "proj-id",
			vd:     newTestViewData(v, start, end, &mean1, &mean2),
			want: []*monitoringpb.CreateTimeSeriesRequest{{
				Name: monitoring.MetricProjectPath("proj-id"),
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-1",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DistributionValue{
									DistributionValue: &distributionpb.Distribution{
										Count: 7,
										Mean:  3.3,
										SumOfSquaredDeviation: 0,
										BucketOptions: &distributionpb.Distribution_BucketOptions{
											Options: &distributionpb.Distribution_BucketOptions_ExplicitBuckets{
												ExplicitBuckets: &distributionpb.Distribution_BucketOptions_Explicit{
													Bounds: []float64{0},
												},
											},
										},
										BucketCounts: []int64{0, 7},
									},
								}},
							},
						},
					},
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-2",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: &monitoredrespb.MonitoredResource{
							Type: "global",
						},
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DistributionValue{
									DistributionValue: &distributionpb.Distribution{
										Count: 5,
										Mean:  -7.7,
										SumOfSquaredDeviation: 0,
										BucketOptions: &distributionpb.Distribution_BucketOptions{
											Options: &distributionpb.Distribution_BucketOptions_ExplicitBuckets{
												ExplicitBuckets: &distributionpb.Distribution_BucketOptions_Explicit{
													Bounds: []float64{0},
												},
											},
										},
										BucketCounts: []int64{0, 5},
									},
								}},
							},
						},
					},
				},
			}},
		},
		{
			name:   "dist agg + time window",
			projID: "proj-id",
			vd:     newTestDistViewData(distView, start, end),
			want:   nil, //TODO: add expectation for distribution
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &statsExporter{
				o:         Options{ProjectID: tt.projID},
				taskValue: taskValue,
			}
			resps := e.makeReq([]*view.Data{tt.vd}, maxTimeSeriesPerUpload)
			if tt.want == nil {
				t.Skip("Missing expectation")
			}
			if got, want := len(resps), len(tt.want); got != want {
				t.Fatalf("%v: Exporter.makeReq() returned %d responses; want %d", tt.name, got, want)
			}
			if len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(resps, tt.want) {
				t.Errorf("%v: Exporter.makeReq() = %v, want %v", tt.name, resps, tt.want)
			}
		})
	}
}

func TestExporter_makeReq_batching(t *testing.T) {
	m, err := stats.Float64("test-measure/makeReq_batching", "measure desc", "unit")
	if err != nil {
		t.Fatal(err)
	}

	key, err := tag.NewKey("test_key")
	if err != nil {
		t.Fatal(err)
	}

	v := &view.View{
		Name:        "view",
		Description: "desc",
		TagKeys:     []tag.Key{key},
		Measure:     m,
		Aggregation: view.Count(),
	}

	tests := []struct {
		name      string
		iter      int
		limit     int
		wantReqs  int
		wantTotal int
	}{
		{
			name:      "4 vds; 3 limit",
			iter:      2,
			limit:     3,
			wantReqs:  2,
			wantTotal: 4,
		},
		{
			name:      "4 vds; 4 limit",
			iter:      2,
			limit:     4,
			wantReqs:  1,
			wantTotal: 4,
		},
		{
			name:      "4 vds; 5 limit",
			iter:      2,
			limit:     5,
			wantReqs:  1,
			wantTotal: 4,
		},
	}

	count1 := view.CountData(10)
	count2 := view.CountData(16)

	for _, tt := range tests {
		var vds []*view.Data
		for i := 0; i < tt.iter; i++ {
			vds = append(vds, newTestViewData(v, time.Now(), time.Now(), &count1, &count2))
		}

		e := &statsExporter{}
		resps := e.makeReq(vds, tt.limit)
		if len(resps) != tt.wantReqs {
			t.Errorf("%v: got %v; want %d requests", tt.name, resps, tt.wantReqs)
		}

		var total int
		for _, resp := range resps {
			total += len(resp.TimeSeries)
		}
		if got, want := total, tt.wantTotal; got != want {
			t.Errorf("%v: len(resps[...].TimeSeries) = %d; want %d", tt.name, got, want)
		}
	}
}

func TestEqualAggWindowTagKeys(t *testing.T) {
	key1, _ := tag.NewKey("test-key-one")
	key2, _ := tag.NewKey("test-key-two")
	tests := []struct {
		name    string
		md      *metricpb.MetricDescriptor
		agg     *view.Aggregation
		keys    []tag.Key
		wantErr bool
	}{
		{
			name: "count agg",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Count(),
			wantErr: false,
		},
		{
			name: "sum agg",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Sum(),
			wantErr: false,
		},
		{
			name: "mean agg",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Mean(),
			wantErr: false,
		},
		{
			name: "distribution agg - mismatch",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Count(),
			wantErr: true,
		},
		{
			name: "mean agg - mismatch",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Mean(),
			wantErr: true,
		},
		{
			name: "distribution agg with keys",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
				Labels: []*label.LabelDescriptor{
					{Key: "test_key_one"},
					{Key: "test_key_two"},
					{Key: opencensusTaskKey},
				},
			},
			agg:     view.Distribution(),
			keys:    []tag.Key{key1, key2},
			wantErr: false,
		},
		{
			name: "distribution agg with keys -- mismatch",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
			},
			agg:     view.Distribution(),
			keys:    []tag.Key{key1, key2},
			wantErr: true,
		},
		{
			name: "count agg with pointers",
			md: &metricpb.MetricDescriptor{
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Labels:     []*label.LabelDescriptor{{Key: opencensusTaskKey}},
			},
			agg:     view.Count(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := equalAggTagKeys(tt.md, tt.agg, tt.keys)
			if err != nil && !tt.wantErr {
				t.Errorf("equalAggTagKeys() = %q; want no error", err)
			}
			if err == nil && tt.wantErr {
				t.Errorf("equalAggTagKeys() = %q; want error", err)
			}

		})
	}
}

func TestExporter_createMeasure(t *testing.T) {
	oldGetMetricDescriptor := getMetricDescriptor
	oldCreateMetricDescriptor := createMetricDescriptor

	defer func() {
		getMetricDescriptor = oldGetMetricDescriptor
		createMetricDescriptor = oldCreateMetricDescriptor
	}()

	key, _ := tag.NewKey("test-key-one")
	m, err := stats.Float64("test-measure/TestExporter_createMeasure", "measure desc", stats.UnitMilliseconds)
	if err != nil {
		t.Fatal(err)
	}

	v := &view.View{
		Name:        "test_view_sum",
		Description: "view_description",
		TagKeys:     []tag.Key{key},
		Measure:     m,
		Aggregation: view.Sum(),
	}

	data := view.CountData(0)
	vd := newTestViewData(v, time.Now(), time.Now(), &data, &data)

	e := &statsExporter{
		createdViews: make(map[string]*metricpb.MetricDescriptor),
	}

	var getCalls, createCalls int
	getMetricDescriptor = func(ctx context.Context, c *monitoring.MetricClient, mdr *monitoringpb.GetMetricDescriptorRequest) (*metric.MetricDescriptor, error) {
		getCalls++
		return nil, status.Error(codes.NotFound, "")
	}
	createMetricDescriptor = func(ctx context.Context, c *monitoring.MetricClient, mdr *monitoringpb.CreateMetricDescriptorRequest) (*metric.MetricDescriptor, error) {
		createCalls++
		if got, want := mdr.MetricDescriptor.Type, "custom.googleapis.com/opencensus/test_view_sum"; got != want {
			t.Errorf("MetricDescriptor.Type = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.ValueType, metricpb.MetricDescriptor_DOUBLE; got != want {
			t.Errorf("MetricDescriptor.ValueType = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.MetricKind, metricpb.MetricDescriptor_CUMULATIVE; got != want {
			t.Errorf("MetricDescriptor.MetricKind = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.Description, "view_description"; got != want {
			t.Errorf("MetricDescriptor.Description = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.DisplayName, "OpenCensus/test_view_sum"; got != want {
			t.Errorf("MetricDescriptor.DisplayName = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.Unit, stats.UnitMilliseconds; got != want {
			t.Errorf("MetricDescriptor.Unit = %q; want %q", got, want)
		}
		return &metric.MetricDescriptor{
			DisplayName: "OpenCensus/test_view_sum",
			Description: "view_description",
			Unit:        stats.UnitMilliseconds,
			Type:        "custom.googleapis.com/opencensus/test_view_sum",
			MetricKind:  metricpb.MetricDescriptor_CUMULATIVE,
			ValueType:   metricpb.MetricDescriptor_DOUBLE,
			Labels:      newLabelDescriptors(vd.View.TagKeys),
		}, nil
	}

	ctx := context.Background()
	if err := e.createMeasure(ctx, vd); err != nil {
		t.Errorf("Exporter.createMeasure() error = %v", err)
	}
	if err := e.createMeasure(ctx, vd); err != nil {
		t.Errorf("Exporter.createMeasure() error = %v", err)
	}
	if count := getCalls; count != 1 {
		t.Errorf("getMetricDescriptor needs to be called for once; called %v times", count)
	}
	if count := createCalls; count != 1 {
		t.Errorf("createMetricDescriptor needs to be called for once; called %v times", count)
	}
	if count := len(e.createdViews); count != 1 {
		t.Errorf("len(e.createdViews) = %v; want 1", count)
	}
}

func TestExporter_createMeasure_CountAggregation(t *testing.T) {
	oldGetMetricDescriptor := getMetricDescriptor
	oldCreateMetricDescriptor := createMetricDescriptor

	defer func() {
		getMetricDescriptor = oldGetMetricDescriptor
		createMetricDescriptor = oldCreateMetricDescriptor
	}()

	key, _ := tag.NewKey("test-key-one")
	m, err := stats.Float64("test-measure/TestExporter_createMeasure", "measure desc", stats.UnitMilliseconds)
	if err != nil {
		t.Fatal(err)
	}

	v := &view.View{
		Name:        "test_view_count",
		Description: "view_description",
		TagKeys:     []tag.Key{key},
		Measure:     m,
		Aggregation: view.Count(),
	}

	data := view.CountData(0)
	vd := newTestViewData(v, time.Now(), time.Now(), &data, &data)

	e := &statsExporter{
		createdViews: make(map[string]*metricpb.MetricDescriptor),
	}

	getMetricDescriptor = func(ctx context.Context, c *monitoring.MetricClient, mdr *monitoringpb.GetMetricDescriptorRequest) (*metric.MetricDescriptor, error) {
		return nil, status.Error(codes.NotFound, "")
	}
	createMetricDescriptor = func(ctx context.Context, c *monitoring.MetricClient, mdr *monitoringpb.CreateMetricDescriptorRequest) (*metric.MetricDescriptor, error) {
		if got, want := mdr.MetricDescriptor.Type, "custom.googleapis.com/opencensus/test_view_count"; got != want {
			t.Errorf("MetricDescriptor.Type = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.ValueType, metricpb.MetricDescriptor_INT64; got != want {
			t.Errorf("MetricDescriptor.ValueType = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.MetricKind, metricpb.MetricDescriptor_CUMULATIVE; got != want {
			t.Errorf("MetricDescriptor.MetricKind = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.Description, "view_description"; got != want {
			t.Errorf("MetricDescriptor.Description = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.DisplayName, "OpenCensus/test_view_count"; got != want {
			t.Errorf("MetricDescriptor.DisplayName = %q; want %q", got, want)
		}
		if got, want := mdr.MetricDescriptor.Unit, stats.UnitNone; got != want {
			t.Errorf("MetricDescriptor.Unit = %q; want %q", got, want)
		}
		return &metric.MetricDescriptor{
			DisplayName: "OpenCensus/test_view_sum",
			Description: "view_description",
			Unit:        stats.UnitNone,
			Type:        "custom.googleapis.com/opencensus/test_view_count",
			MetricKind:  metricpb.MetricDescriptor_CUMULATIVE,
			ValueType:   metricpb.MetricDescriptor_INT64,
			Labels:      newLabelDescriptors(vd.View.TagKeys),
		}, nil
	}
	ctx := context.Background()
	if err := e.createMeasure(ctx, vd); err != nil {
		t.Errorf("Exporter.createMeasure() error = %v", err)
	}
}

func TestExporter_makeReq_withCustomMonitoredResource(t *testing.T) {
	m, err := stats.Float64("test-measure/TestExporter_makeReq_withCustomMonitoredResource", "measure desc", "unit")
	if err != nil {
		t.Fatal(err)
	}

	key, err := tag.NewKey("test_key")
	if err != nil {
		t.Fatal(err)
	}

	v := &view.View{
		Name:        "testview",
		Description: "desc",
		TagKeys:     []tag.Key{key},
		Measure:     m,
		Aggregation: view.Count(),
	}
	if err := view.Subscribe(v); err != nil {
		t.Fatal(err)
	}
	defer view.Unsubscribe(v)

	start := time.Now()
	end := start.Add(time.Minute)
	count1 := view.CountData(10)
	count2 := view.CountData(16)
	taskValue := getTaskValue()

	resource := &monitoredrespb.MonitoredResource{
		Type:   "gce_instance",
		Labels: map[string]string{"instance_id": "instance", "zone": "us-west-1a"},
	}

	tests := []struct {
		name   string
		projID string
		vd     *view.Data
		want   []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			name:   "count agg timeline",
			projID: "proj-id",
			vd:     newTestViewData(v, start, end, &count1, &count2),
			want: []*monitoringpb.CreateTimeSeriesRequest{{
				Name: monitoring.MetricProjectPath("proj-id"),
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-1",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: resource,
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{
									Int64Value: 10,
								}},
							},
						},
					},
					{
						Metric: &metricpb.Metric{
							Type: "custom.googleapis.com/opencensus/testview",
							Labels: map[string]string{
								"test_key":        "test-value-2",
								opencensusTaskKey: taskValue,
							},
						},
						Resource: resource,
						Points: []*monitoringpb.Point{
							{
								Interval: &monitoringpb.TimeInterval{
									StartTime: &timestamp.Timestamp{
										Seconds: start.Unix(),
										Nanos:   int32(start.Nanosecond()),
									},
									EndTime: &timestamp.Timestamp{
										Seconds: end.Unix(),
										Nanos:   int32(end.Nanosecond()),
									},
								},
								Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{
									Int64Value: 16,
								}},
							},
						},
					},
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &statsExporter{
				o:         Options{ProjectID: tt.projID, Resource: resource},
				taskValue: taskValue,
			}
			resps := e.makeReq([]*view.Data{tt.vd}, maxTimeSeriesPerUpload)
			if got, want := len(resps), len(tt.want); got != want {
				t.Fatalf("%v: Exporter.makeReq() returned %d responses; want %d", tt.name, got, want)
			}
			if len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(resps, tt.want) {
				t.Errorf("%v: Exporter.makeReq() = %v, want %v", tt.name, resps, tt.want)
			}
		})
	}
}

func newTestViewData(v *view.View, start, end time.Time, data1, data2 view.AggregationData) *view.Data {
	key, _ := tag.NewKey("test-key")
	tag1 := tag.Tag{Key: key, Value: "test-value-1"}
	tag2 := tag.Tag{Key: key, Value: "test-value-2"}
	return &view.Data{
		View: v,
		Rows: []*view.Row{
			{
				Tags: []tag.Tag{tag1},
				Data: data1,
			},
			{
				Tags: []tag.Tag{tag2},
				Data: data2,
			},
		},
		Start: start,
		End:   end,
	}
}

func newTestDistViewData(v *view.View, start, end time.Time) *view.Data {
	return &view.Data{
		View: v,
		Rows: []*view.Row{
			{Data: &view.DistributionData{
				Count:           5,
				Min:             1,
				Max:             7,
				Mean:            3,
				SumOfSquaredDev: 1.5,
				CountPerBucket:  []int64{2, 2, 1},
			}},
		},
		Start: start,
		End:   end,
	}
}
