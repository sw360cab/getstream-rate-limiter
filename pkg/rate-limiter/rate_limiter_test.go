package rate_limiter

import (
	"fmt"
	"math"
	"testing"
	"time"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	var start time.Time
	logger, _ := test.NewNullLogger()

	type rateLimtedCase struct {
		name           string
		iterationCount int
		resetSeconds   int64
	}

	tests := []rateLimtedCase{
		{
			name:           "Non-Blocking iteration",
			iterationCount: 1,
			resetSeconds:   0,
		},
		{
			name:           "Block after first blocking iteration",
			iterationCount: 2,
			resetSeconds:   4,
		},
		{
			name:           "Several iterations and wait",
			iterationCount: 5,
			resetSeconds:   3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rLimit := RateLimiter{
				apiName: tt.name,
				token:   make(chan struct{}, 1),
			}

			mockResponse := &stream.Response{
				RateLimitInfo: &stream.RateLimitInfo{
					Remaining: int64(tt.iterationCount - 1),
					Reset:     time.Now().Unix() + tt.resetSeconds,
				},
			}

			for i := 0; i < tt.iterationCount; i++ {
				start = time.Now()
				rLimit.CallApiAndBlockOnRateLimit(logger, func() (resp *stream.Response, err error) {
					mockResponse.RateLimitInfo.Remaining -= 1
					return mockResponse, nil
				})
			}

			since := time.Since(start)
			assert.Greater(t, since, time.Duration((tt.resetSeconds-1)*int64(math.Pow10(9))))
			assert.Less(t, since, time.Duration((tt.resetSeconds+1)*int64(4*math.Pow10(9))))
			fmt.Printf("Time passed %v\n", time.Since(start))
		})
	}
}

func TestRateLimitErrorInApiCall(t *testing.T) {
	logger, _ := test.NewNullLogger()

	tests := []struct {
		name      string
		mockFn    GetStreamApiCaller
		wantError assert.ErrorAssertionFunc
	}{
		{
			name: "Mocked no Error",
			mockFn: func() (resp *stream.Response, err error) {
				return &stream.Response{
					RateLimitInfo: &stream.RateLimitInfo{
						Remaining: 1,
						Reset:     time.Now().Unix(),
					},
				}, nil
			},
			wantError: assert.NoError,
		},
		{
			name: "Mocked with an Error",
			mockFn: func() (resp *stream.Response, err error) {
				return nil, assert.AnError
			},
			wantError: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rLimit := RateLimiter{
				token: make(chan struct{}, 1),
			}

			tt.wantError(t, rLimit.CallApiAndBlockOnRateLimit(logger, tt.mockFn))
		})
	}
}
