package prebid_cache_client

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/prebid/prebid-server/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Prevents #197
func TestEmptyPut(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("The server should not be called.")
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := &clientImpl{
		httpClient: server.Client(),
		putUrl:     server.URL,
	}
	ids, _ := client.PutJson(context.Background(), nil)
	assertIntEqual(t, len(ids), 0)
	ids, _ = client.PutJson(context.Background(), []Cacheable{})
	assertIntEqual(t, len(ids), 0)
}

func TestBadResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := &clientImpl{
		httpClient: server.Client(),
		putUrl:     server.URL,
	}
	ids, _ := client.PutJson(context.Background(), []Cacheable{
		{
			Type: TypeJSON,
			Data: json.RawMessage("true"),
		}, {
			Type: TypeJSON,
			Data: json.RawMessage("false"),
		},
	})
	assertIntEqual(t, len(ids), 2)
	assertStringEqual(t, ids[0], "")
	assertStringEqual(t, ids[1], "")
}

func TestCancelledContext(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := &clientImpl{
		httpClient: server.Client(),
		putUrl:     server.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ids, _ := client.PutJson(ctx, []Cacheable{{
		Type: TypeJSON,
		Data: json.RawMessage("true"),
	},
	})
	assertIntEqual(t, len(ids), 1)
	assertStringEqual(t, ids[0], "")
}

func TestSuccessfulPut(t *testing.T) {
	server := httptest.NewServer(newHandler(2))
	defer server.Close()

	client := &clientImpl{
		httpClient: server.Client(),
		putUrl:     server.URL,
	}

	ids, _ := client.PutJson(context.Background(), []Cacheable{
		{
			Type:       TypeJSON,
			Data:       json.RawMessage("true"),
			TTLSeconds: 300,
		}, {
			Type: TypeJSON,
			Data: json.RawMessage("false"),
		},
	})
	assertIntEqual(t, len(ids), 2)
	assertStringEqual(t, ids[0], "0")
	assertStringEqual(t, ids[1], "1")
}

func TestEncodeValueToBuffer(t *testing.T) {
	buf := new(bytes.Buffer)
	testCache := Cacheable{
		Type:       TypeJSON,
		Data:       json.RawMessage(`{}`),
		TTLSeconds: 300,
	}
	expected := string(`{"type":"json","ttlseconds":300,"value":{}}`)
	_ = encodeValueToBuffer(testCache, false, buf)
	actual := buf.String()
	assertStringEqual(t, expected, actual)
}

func TestStripCacheHostAndPath(t *testing.T) {
	type aTest struct {
		inConfig     []byte
		expectedHost string
		expectedPath string
	}
	testInput := []aTest{
		{inConfig: []byte(`
cache:
  scheme: http
  host: prebid-server.prebid.org
  path: pbcache/endpoint
`), expectedHost: "prebid-server.prebid.org", expectedPath: "pbcache/endpoint"},
		{inConfig: []byte(`
cache:
  scheme: http
  host: prebidcache.net
  query: uuid=%PBS_CACHE_UUID%
`), expectedHost: "prebidcache.net", expectedPath: "cache"},
		{inConfig: []byte(`
cache:
  scheme: http
  host: prebid-server.prebid.org
`), expectedHost: "prebid-server.prebid.org", expectedPath: "cache"},
		{inConfig: []byte(``), expectedHost: "", expectedPath: ""},
	}
	for i, test := range testInput {
		//start viper
		v := viper.New()
		config.SetupViper(v, "")
		v.SetConfigType("yaml")
		//parse testst config into a config object
		v.ReadConfig(bytes.NewBuffer(test.inConfig))
		cfg, _ := config.New(v)

		//start client
		cacheClient := NewClient(&cfg.CacheURL)
		cHost, cPath := cacheClient.GetPrebidCacheSplitURL()

		//assert
		assert.Equal(t, test.expectedHost, cHost, "Expected host '%s', got '%s' \n", i+1, test.expectedHost, cHost)
		assert.Equal(t, test.expectedPath, cPath, "Expected path '%s', got '%s' \n", i+1, test.expectedPath, cPath)
	}
}

func assertIntEqual(t *testing.T, expected, actual int) {
	t.Helper()
	if expected != actual {
		t.Errorf("Expected %d, got %d", expected, actual)
	}
}

func assertStringEqual(t *testing.T, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf(`Expected "%s", got "%s"`, expected, actual)
	}
}

func newHandler(numResponses int) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := response{
			Responses: make([]responseObject, numResponses),
		}
		for i := 0; i < numResponses; i++ {
			resp.Responses[i].UUID = strconv.Itoa(i)
		}

		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	})
}
