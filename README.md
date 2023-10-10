# GetStream Rate Limiter

> Handling Rate Limits in GetStream API Calls in Go leveraging Channels and Goroutines

This module is my own solution to overcome rate limits for GetStream's API Endpoints when employing Go SDK.

Check reasons for creating this in my [blog post](https://dev.to/sw360cab/handling-rate-limits-in-getstream-api-calls-in-go-leveraging-channels-and-goroutines-29a4)

## Installing

Use `go get` to install the latest version of the library.

```bash
go get -u github.com/sw360cab/getstream-rate-limiter
```

Next, include it in your application:

```go
import "github.com/sw360cab/getstream-rate-limitera"
```

## Usage

Given a `GetStream` chat client:

```go
  getStreamChatClient, err := stream.NewClient("<API_KEY>", "<API_SECRET>")
```

and a `RateLimiter` struct exposed by this module, e.g. in a map having as key type a GetStream Endpoint:

```go
type GetStreamApiName string

const (
  CreateChannel GetStreamApiName = "CreateChannel"
  QueryChannel  GetStreamApiName = "QueryChannel"
)

rateLimiterMap := map[GetStreamApiName]RateLimiter{
  CreateChannel: {
    apiName: string(CreateChannel),
    token:   make(chan struct{}, 1),
  },
  QueryChannel: {
    apiName: string(CreateChannel),
    token:   make(chan struct{}, 1),
  },
}
```

a method to call GetStream `QueryChannel` Api, leveraging `RateLimiter`, can be defined as follows:

```go
func queryChannels(filters map[string]any) (queryResp *stream.QueryChannelsResponse, err error) {
  rateLimiter := rateLimiterMap["QueryChannel"]
  if errCall := rateLimiter.CallApiAndBlockOnRateLimit(logger, func() (resp *stream.Response, err error) {
    queryResp, err = getStreamChatClient.QueryChannels(context.Background(),
      &stream.QueryOption{
        Filter: filters,
      },
    )
    if err != nil {
      return nil, err
    }
    return &queryResp.Response, nil
  });
  errCall != nil {
    return nil, errCall
  }
  return queryResp, nil
}
```
