package statsig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries        = 5
	backoffMultiplier = 10
)

type statsigMetadata struct {
	SDKType    string `json:"sdkType"`
	SDKVersion string `json:"sdkVersion"`
}

type transport struct {
	api      string
	sdkKey   string
	metadata statsigMetadata
	client   *http.Client
	options  *Options
}

func newTransport(secret string, options *Options) *transport {
	api := defaultString(options.API, DefaultEndpoint)
	api = strings.TrimSuffix(api, "/")

	return &transport{
		api:      api,
		metadata: statsigMetadata{SDKType: "go-sdk", SDKVersion: "1.1.0"},
		sdkKey:   secret,
		client:   &http.Client{},
		options:  options,
	}
}

func (transport *transport) postRequest(
	endpoint string,
	in interface{},
	out interface{},
) error {
	return transport.postRequestInternal(endpoint, in, out, 0, 0)
}

func (transport *transport) retryablePostRequest(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
) error {
	return transport.postRequestInternal(endpoint, in, out, retries, time.Second)
}

func (transport *transport) postRequestInternal(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
	backoff time.Duration,
) error {
	if transport.options.LocalMode {
		return nil
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return retry(retries, time.Duration(backoff), func() (bool, error) {
		req, err := http.NewRequest("POST", transport.api+endpoint, bytes.NewBuffer(body))
		if err != nil {
			return false, err
		}

		req.Header.Add("STATSIG-API-KEY", transport.sdkKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Add("STATSIG-CLIENT-TIME", strconv.FormatInt(time.Now().Unix()*1000, 10))

		response, err := transport.client.Do(req)
		if err != nil {
			return true, err
		}
		defer response.Body.Close()

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return false, json.NewDecoder(response.Body).Decode(&out)
		}

		return shouldRetry(response.StatusCode), fmt.Errorf("http response error code: %d", response.StatusCode)
	})
}

func retry(retries int, backoff time.Duration, fn func() (bool, error)) error {
	for {
		if retry, err := fn(); retry {
			if retries <= 0 {
				return err
			}

			retries--
			time.Sleep(backoff)
			backoff = backoff * backoffMultiplier
		} else {
			return err
		}
	}
}

func shouldRetry(code int) bool {
	switch code {
	case 408, 500, 502, 503, 504, 522, 524, 599:
		return true
	default:
		return false
	}
}
