package v3api

import (
    "context"
    "testing"
    "time"

    api_v3 "github.com/jaegertracing/jaeger-idl/proto-gen/api_v3"
    modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
    "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    "go.uber.org/zap"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "google.golang.org/protobuf/types/known/durationpb"
    "google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetDependencies_Success(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    endTime := time.Now()
    lookback := 24 * time.Hour

    expectedDeps := []modelv1.DependencyLink{
        {
            Parent:    "frontend",
            Child:     "backend",
            CallCount: 100,
            Source:    "traces",
        },
        {
            Parent:    "backend",
            Child:     "database",
            CallCount: 500,
            Source:    "traces",
        },
    }

    mockReader.On("GetDependencies",
        mock.Anything,
        mock.MatchedBy(func(t time.Time) bool {
            return t.Unix() == endTime.Unix()
        }),
        lookback,
    ).Return(expectedDeps, nil)

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(endTime),
        Lookback: durationpb.New(lookback),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.NoError(t, err)
    require.NotNil(t, resp)
    assert.Len(t, resp.Dependencies, 2)
    
    assert.Equal(t, "frontend", resp.Dependencies[0].Parent)
    assert.Equal(t, "backend", resp.Dependencies[0].Child)
    assert.Equal(t, uint64(100), resp.Dependencies[0].CallCount)
    assert.Equal(t, "traces", resp.Dependencies[0].Source)
    
    assert.Equal(t, "backend", resp.Dependencies[1].Parent)
    assert.Equal(t, "database", resp.Dependencies[1].Child)
    assert.Equal(t, uint64(500), resp.Dependencies[1].CallCount)

    mockReader.AssertExpectations(t)
}

func TestGetDependencies_EmptyResult(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    endTime := time.Now()
    lookback := 1 * time.Hour

    mockReader.On("GetDependencies",
        mock.Anything,
        mock.Anything,
        mock.Anything,
    ).Return([]modelv1.DependencyLink{}, nil)

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(endTime),
        Lookback: durationpb.New(lookback),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.NoError(t, err)
    require.NotNil(t, resp)
    assert.Empty(t, resp.Dependencies)
}

func TestGetDependencies_MissingEndTime(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    req := &api_v3.GetDependenciesRequest{
        EndTime:  nil,
        Lookback: durationpb.New(24 * time.Hour),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.InvalidArgument, st.Code())
    assert.Contains(t, st.Message(), "end_time is required")
}

func TestGetDependencies_MissingLookback(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(time.Now()),
        Lookback: nil,
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.InvalidArgument, st.Code())
    assert.Contains(t, st.Message(), "lookback is required")
}

func TestGetDependencies_NegativeLookback(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(time.Now()),
        Lookback: durationpb.New(-24 * time.Hour),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.InvalidArgument, st.Code())
    assert.Contains(t, st.Message(), "lookback must be a positive duration")
}

func TestGetDependencies_FutureEndTime(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    futureTime := time.Now().Add(2 * time.Hour)
    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(futureTime),
        Lookback: durationpb.New(24 * time.Hour),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.InvalidArgument, st.Code())
    assert.Contains(t, st.Message(), "future")
}

func TestGetDependencies_StorageError(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    endTime := time.Now()
    lookback := 24 * time.Hour

    mockReader.On("GetDependencies",
        mock.Anything,
        mock.Anything,
        mock.Anything,
    ).Return(nil, assert.AnError)

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(endTime),
        Lookback: durationpb.New(lookback),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.Internal, st.Code())
}

func TestGetDependencies_TimeRangeBeforeEpoch(t *testing.T) {
    mockReader := &mocks.Reader{}
    logger := zap.NewNop()
    handler := NewHandler(HandlerOptions{
        DependencyReader: mockReader,
        Logger:           logger,
    })

    // Set end time very close to epoch with a large lookback
    endTime := time.Unix(100, 0)
    lookback := 200 * time.Second

    req := &api_v3.GetDependenciesRequest{
        EndTime:  timestamppb.New(endTime),
        Lookback: durationpb.New(lookback),
    }

    resp, err := handler.GetDependencies(context.Background(), req)

    require.Error(t, err)
    assert.Nil(t, resp)
    
    st, ok := status.FromError(err)
    require.True(t, ok)
    assert.Equal(t, codes.InvalidArgument, st.Code())
    assert.Contains(t, st.Message(), "Unix epoch")
}
