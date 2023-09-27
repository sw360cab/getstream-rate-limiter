package rate_limiter

import (
	"time"

	stream "github.com/GetStream/stream-chat-go/v6"
	log "github.com/sirupsen/logrus"
)

type GetStreamApiCaller func() (resp *stream.Response, err error)

type GetStreamApiName string

const (
	CreateChannel GetStreamApiName = "CreateChannel"
	QueryChannel  GetStreamApiName = "QueryChannel"
	QueryUsers    GetStreamApiName = "QueryUsers"
)

type RateLimiter struct {
	apiName string
	token   chan struct{}
}

// --> Single Slot Channel + Sleep [more performant]
func (r *RateLimiter) CallApiAndBlockOnRateLimit(logger *log.Logger, apiCall GetStreamApiCaller) error {
	r.token <- struct{}{}
	// Alt. Direct API call in GetStream <-- requires network traffic
	// resp, err := r.client.GetRateLimits(context.TODO(), WithEndpoints(r.apiName))

	// Injected api call
	resp, err := apiCall()
	if err != nil {
		<-r.token
		return err
	}
	logger.Tracef("After api call for %s, remaining api calls %d/%d\n", r.apiName, resp.RateLimitInfo.Remaining, resp.RateLimitInfo.Limit)
	if resp.RateLimitInfo.Remaining == 0 {
		logger.Debugf("No more call left for %s.\n", r.apiName)
		go func(duration int64) {
			start := time.Now()
			logger.Debugf("Blocking future calls of %s for %d seconds\n", r.apiName, time.Duration(duration-start.Unix()))
			time.Sleep((time.Second * time.Duration(duration-start.Unix())).Abs())
			<-r.token
			logger.Tracef("Restarting api %s after %f seconds at %v\n", r.apiName, time.Since(start).Seconds(), time.Now().UTC())
		}(resp.RateLimitInfo.Reset) // <-- when the current limit will reset (Unix timestamp in seconds)
	} else {
		<-r.token
	}
	return nil
}
