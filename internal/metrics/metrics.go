// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// MustInit initializes the passed in metrics and initializes its fields using the passed in factory.
//
// It uses reflection to initialize a struct containing metrics fields
// by assigning new Counter/Gauge/Timer values with the metric name retrieved
// from the `metric` tag and stats tags retrieved from the `tags` tag.
//
// Note: all fields of the struct must be exported, have a `metric` tag, and be
// of type Counter or Gauge or Timer.
//
// Errors during Init lead to a panic.
func MustInit(metrics any, factory Factory, globalTags map[string]string) {
	if err := Init(metrics, factory, globalTags); err != nil {
		panic(err.Error())
	}
}

// Init does the same as MustInit, but returns an error instead of
// panicking.
func Init(m any, factory Factory, globalTags map[string]string) error {
	// Allow user to opt out of reporting metrics by passing in nil.
	if factory == nil {
		factory = NullFactory
	}

	v := reflect.ValueOf(m).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tags := make(map[string]string)
		maps.Copy(tags, globalTags)

		field := t.Field(i)
		metric := field.Tag.Get("metric")
		if metric == "" {
			return fmt.Errorf("Field %s is missing a tag 'metric'", field.Name)
		}
		if tagString := field.Tag.Get("tags"); tagString != "" {
			tagPairs := strings.Split(tagString, ",")
			for _, pair := range tagPairs {
				kv := strings.Split(pair, "=")
				if len(kv) == 2 {
					tags[kv[0]] = kv[1]
				}
			}
		}
		help := field.Tag.Get("help")

		var buckets []float64
		var durationBuckets []time.Duration
		var err error

		bucketTag := field.Tag.Get("buckets")

		switch {
		case field.Type.AssignableTo(timerPtrType):
			if bucketTag != "" {
				durationBuckets, err = parseBuckets(bucketTag, time.ParseDuration)
				if err != nil {
					return fmt.Errorf("Field [%s]: %w", field.Name, err)
				}
			}
		case field.Type.AssignableTo(histogramPtrType):
			if bucketTag != "" {
				buckets, err = parseBuckets(bucketTag, func(s string) (float64, error) {
					return strconv.ParseFloat(s, 64)
				})
				if err != nil {
					return fmt.Errorf("Field [%s]: %w", field.Name, err)
				}
			}
		default:
		}

		var obj any
		switch {
		case field.Type.AssignableTo(counterPtrType):
			obj = factory.Counter(Options{
				Name: metric,
				Tags: tags,
				Help: help,
			})
		case field.Type.AssignableTo(gaugePtrType):
			obj = factory.Gauge(Options{
				Name: metric,
				Tags: tags,
				Help: help,
			})
		case field.Type.AssignableTo(timerPtrType):
			obj = factory.Timer(TimerOptions{
				Name:    metric,
				Tags:    tags,
				Help:    help,
				Buckets: durationBuckets,
			})
		case field.Type.AssignableTo(histogramPtrType):
			obj = factory.Histogram(HistogramOptions{
				Name:    metric,
				Tags:    tags,
				Help:    help,
				Buckets: buckets,
			})
		case field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct:
			nestedMetrics := reflect.New(field.Type.Elem()).Interface()
			// Explicitly using NSOptions here
			if err := Init(nestedMetrics, factory.Namespace(NSOptions{
				Name: metric,
				Tags: tags,
			}), nil); err != nil {
				return err
			}
			obj = nestedMetrics
		default:
			return fmt.Errorf(
				"Field %s is not a pointer to timer, gauge, counter, histogram or struct",
				field.Name)
		}
		if obj != nil {
			v.Field(i).Set(reflect.ValueOf(obj))
		}
	}
	return nil
}

func parseBuckets[T any](tag string, parse func(string) (T, error)) ([]T, error) {
	if tag == "" {
		return nil, nil
	}
	parts := strings.Split(tag, ",")
	results := make([]T, 0, len(parts))
	for _, part := range parts {
		val, err := parse(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("bucket [%s] could not be parsed: %w", part, err)
		}
		results = append(results, val)
	}
	return results, nil
}

var (
	counterPtrType   = reflect.TypeOf((*Counter)(nil)).Elem()
	gaugePtrType     = reflect.TypeOf((*Gauge)(nil)).Elem()
	timerPtrType     = reflect.TypeOf((*Timer)(nil)).Elem()
	histogramPtrType = reflect.TypeOf((*Histogram)(nil)).Elem()
)
