# Handling Rate Limits in GetStream API Calls in Go leveraging Channels and Goroutines

TL; DR

This is my own solution to overcome rate limits for GetStream's API Endpoints when employing Go SDK.

[GetStream](https://getstream.io/) is a great SaaS offering Chat & Feed services and a wide variety of SDKs acting as clients to handle those items and their users as well.
Speaking of chats in GetStream, I faced a severe problem in one of the projects I worked on: [Rate Limits in API calls](https://getstream.io/chat/docs/go-golang/rate_limits/?language=go).
Indeed GetStream requires the various clients to perform their requests respecting a specific rate limiting in requesting the endpoints available, such as `QueryChannels`, `QueryUsers`, `CreateChannel`. Limits for the different endpoints are listed [here](https://getstream.io/chat/docs/go-golang/rate_limits_table/?language=go)

It is not easy to deal with these boundaries, especially when you create batch tasks, because we tend to imagine and code big `loops` to apply multiple requests to a set of homogeneous data.
In my case, in  the startup I was working for, we had a task to handle a large set of groups, each corresponding to a chat group in GetStream; participants in these groups used to be added or removed by a sort of group's administrator. Since this might not be reflected real-time in the chat groups, there was a nightly task that basically checks for each user whether he should be added to or removed from a chat group.

> Probably watching the problem upside down, it would be more clever to check all the existing chat groups instead of every user, but this is not the point here. Let's suppose this is the best implementation we could use.

Sometimes the task was limited to a small bunch of changes, but most of the time many users were repeatedly added and removed from groups. And that made the task failing often because after several requests it started to time out over and over for all the following ones.

In order to find a solution there are a couple things to take into account:

* Get Stream is setting in the response the [headers](https://getstream.io/chat/docs/go-golang/rate_limits/?language=go#rate-limit-headers) that specify for each kind of api (remember that limits are tied to each specific endpoint): which are the defined limits (`X-RateLimit-Limit`), how many requests are still available (`X-RateLimit-Remaining`) and when eventually the block for hitting rate limit will be reset (`X-RateLimit-Timestamp`) in Unix timestamp format.
* The task needs a way to distinguish among each of all the (employed) Api endpoints, whether the timeout has been reached, and in case it is, a way to hold all the following requests for a given time.

All of that makes me think about generating a sort of barrier for the Api requests that will arrive after the rate limit for the given endpoint is hit. There should be multiple barriers, one for each endpoint and they should be able to block following requests for a specific amount of time given. I named that `timed-barrier`.

Being inside a Go environment I guessed that **Channels** and **Goroutines** would be a natural means to deal with the issue. I achieved distinguishing among the different Api endpoints creating a Timed-barrier reference struct.

```go
type RateLimiter struct {
  apiName string
  token   chan struct{}
}
```

The `RateLimiter` struct represents the Timed-barrier having an Api Endpoint reference and a buffered channel with capacity 1.

☠️ Initially I thought about an unbuffered channel. Since the scenario is not aiming to coordinate two Goroutines against each other but a group of them, it would have created an immediate deadlock.

The buffered channel will be the trick to overcome the rate limits issue. Indeed the first call will occupy the single slot available in the channel and it will release it after calling the API, depending on the return value of current rate limits. If the value of the remaining calls is greater than zero, it will release the channel's slot immediately, otherwise it will wait until the timeout has expired.

But let's see this in details by analyzing the RateLimiter's method `CallApiAndBlockOnRateLimit`:

```go
func (r *RateLimiter) CallApiAndBlockOnRateLimit(logger *log.Logger, apiCall GetStreamApiCaller) error {
  r.token <- struct{}{}
  // Injected api call
  resp, err := apiCall()
  if err != nil {
    <-r.token
    return err
  }
  if resp.RateLimitInfo.Remaining == 0 {
    go func(duration int64) {
      start := time.Now()
      time.Sleep((time.Second * time.Duration(duration-start.Unix())).Abs())
      <-r.token
    }(resp.RateLimitInfo.Reset) // <-- when the current limit will reset (Unix timestamp in seconds)
  } else {
    <-r.token
  }
  return nil
}
```

The first step is occupying the single slot in the buffered channel. It means that any subsequent call will be blocked waiting for an available slot in the channel.
The Api endpoint is called leveraging a clojure or higher-order function type, such as

```go
type GetStreamApiCaller func() (resp *stream.Response, err error)
```

that is basically supposed to call a given GetStream endpoint, which would place the response in a specific type, according to the Api called. The role of the function is extracting a general data structure from the specific response type.
That general response type is `stream.Response` which is defined as follows:

```go
type Response struct {
  RateLimitInfo *RateLimitInfo `json:"ratelimit"`
}

type RateLimitInfo struct {
  // Limit is the maximum number of API calls for a single time window (1 minute).
  Limit int64 `json:"limit"`
  // Remaining is the number of API calls remaining in the current time window (1 minute).
  Remaining int64 `json:"remaining"`
  // Reset is the Unix timestamp of the expiration of the current rate limit time window.
  Reset int64 `json:"reset"`
}
```

It is clear that this is exactly the key of the method implementation.
The `RateLimitInfo` contains all the useful information to proceed.
The `Remaining` field will inform if any call is still available for the given Api endpoint. If it is, the channel is freed immediately, otherwise the channel will remain occupied until a specific time is passed. This is achieved leveraging the `Reset` field which will hold the slot within a Goroutine, until the given time (duration given as Unix timestamp in seconds) has passed, by just _sleeping_.
Moreover even if in a group of homogeneous and distinguished calls order may not be important, blocking all the subsequent calls waiting for the slot of the channel to be empty, will guarantee consistency in the order in which they will be called after the channel slot is released.

All you have to do is create a distinct `RateLimiter` object for each kind of Api Endpoint in GetStream.

## Library in action

In a real world scenario, given a `GetStream` chat client:

```go
  getStreamChatClient, err := stream.NewClient("<API_KEY>", "<API_SECRET>")
```

And a distinct set of RateLimiter objects, collected in a map having as key type a GetStream Api Endpoint:

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

A method to call GetStream `QueryChannel` Api, leveraging `RateLimiter`, can be defined as following:

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

🚀 That's it! I hope you like the way I faced the issue. Feel free to comment or propose your own idea to deal with this.

⚔️ In case anyone finds it useful I packed all of the above together in a [Github repo](https://github.com/sw360cab/getstream-rate-limiter).
You should be able to use it also as module in your code using:

```bash
go get -u github.com/sw360cab/getstream-rate-limiter
```
