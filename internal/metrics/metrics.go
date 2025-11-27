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
// It uses reflection to find all the members of the struct and their tags. It then
// uses the factory to initialize the metrics and assigns the pointers to the struct fields.
// It panics if there is an error initializing the metrics.
func MustInit(m any, factory Factory, globalTags map[string]string) {
	if err := Init(m, factory, globalTags); err != nil {
		panic(err)
	}
}

var (
	counterPtrType   = reflect.TypeOf((*Counter)(nil)).Elem()
	gaugePtrType     = reflect.TypeOf((*Gauge)(nil)).Elem()
	timerPtrType     = reflect.TypeOf((*Timer)(nil)).Elem()
	histogramPtrType = reflect.TypeOf((*Histogram)(nil)).Elem()
)

func parseBuckets[T any](bucketString, fieldName, typeName string, parse func(string) (T, error)) ([]T, error) {
	bucketValues := strings.Split(bucketString, ",")
	buckets := make([]T, 0, len(bucketValues))
	for _, bucket := range bucketValues {
		val, err := parse(bucket)
		if err != nil {
			return nil, fmt.Errorf(
				"Field [%s]: Bucket [%s] could not be converted to %s in 'buckets' string [%s]",
				fieldName, bucket, typeName, bucketString)
		}
		buckets = append(buckets, val)
	}
	return buckets, nil
}

// Init initializes the passed in metrics and initializes its fields using the passed in factory.
// It uses reflection to find all the members of the struct and their tags. It then
// uses the factory to initialize the metrics and assigns the pointers to the struct fields.
func Init(m any, factory Factory, globalTags map[string]string) error {
	if factory == nil {
		factory = NullFactory
	}

	v := reflect.ValueOf(m).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tags := make(map[string]string)
		maps.Copy(tags, globalTags)
		var buckets []float64
		var timerBuckets []time.Duration
		field := t.Field(i)
		metric := field.Tag.Get("metric")
		if metric == "" {
			return fmt.Errorf("Field %s is missing a tag 'metric'", field.Name)
		}
		if tagString := field.Tag.Get("tags"); tagString != "" {
			for tagPair := range strings.SplitSeq(tagString, ",") {
				tag := strings.Split(tagPair, "=")
				if len(tag) != 2 {
					return fmt.Errorf(
						"Field [%s]: Tag [%s] is not of the form key=value in 'tags' string [%s]",
						field.Name, tagPair, tagString)
				}
				tags[tag[0]] = tag[1]
			}
		}
		if bucketString := field.Tag.Get("buckets"); bucketString != "" {
			switch {
			case field.Type.AssignableTo(timerPtrType):
				var err error
				timerBuckets, err = parseBuckets(bucketString, field.Name, "duration", time.ParseDuration)
				if err != nil {
					return err
				}
			case field.Type.AssignableTo(histogramPtrType):
				var err error
				buckets, err = parseBuckets(bucketString, field.Name, "float64", func(s string) (float64, error) {
					return strconv.ParseFloat(s, 64)
				})
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf(
					"Field [%s]: Buckets should only be defined for Timer and Histogram metric types",
					field.Name)
			}
		}
		help := field.Tag.Get("help")
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
				Buckets: timerBuckets,
			})
		case field.Type.AssignableTo(histogramPtrType):
			obj = factory.Histogram(HistogramOptions{
				Name:    metric,
				Tags:    tags,
				Help:    help,
				Buckets: buckets,
			})
		default:
			return fmt.Errorf(
				"Field %s is not a pointer to timer, gauge, or counter",
				field.Name)
		}
		v.Field(i).Set(reflect.ValueOf(obj))
	}
	return nil
}
