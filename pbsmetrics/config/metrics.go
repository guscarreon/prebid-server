package config

import (
	"net/http/httptrace"
	"time"

	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/openrtb_ext"
	"github.com/prebid/prebid-server/pbsmetrics"
	prometheusmetrics "github.com/prebid/prebid-server/pbsmetrics/prometheus"
	metrics "github.com/rcrowley/go-metrics"
	influxdb "github.com/vrischmann/go-metrics-influxdb"
)

// NewMetricsEngine reads the configuration and returns the appropriate metrics engine
// for this instance.
func NewMetricsEngine(cfg *config.Configuration, adapterList []openrtb_ext.BidderName) *DetailedMetricsEngine {
	// Create a list of metrics engines to use.
	// Capacity of 2, as unlikely to have more than 2 metrics backends, and in the case
	// of 1 we won't use the list so it will be garbage collected.
	engineList := make(MultiMetricsEngine, 0, 2)
	returnEngine := DetailedMetricsEngine{}

	if cfg.Metrics.Influxdb.Host != "" {
		// Currently use go-metrics as the metrics piece for influx
		returnEngine.GoMetrics = pbsmetrics.NewMetrics(metrics.NewPrefixedRegistry("prebidserver."), adapterList, cfg.Metrics.Disabled)
		engineList = append(engineList, returnEngine.GoMetrics)
		// Set up the Influx logger
		go influxdb.InfluxDB(
			returnEngine.GoMetrics.MetricsRegistry,                             // metrics registry
			time.Second*time.Duration(cfg.Metrics.Influxdb.MetricSendInterval), // Configurable interval
			cfg.Metrics.Influxdb.Host,                                          // the InfluxDB url
			cfg.Metrics.Influxdb.Database,                                      // your InfluxDB database
			cfg.Metrics.Influxdb.Username,                                      // your InfluxDB user
			cfg.Metrics.Influxdb.Password,                                      // your InfluxDB password
		)
		// Influx is not added to the engine list as goMetrics takes care of it already.
	}
	if cfg.Metrics.Prometheus.Port != 0 {
		// Set up the Prometheus metrics.
		returnEngine.PrometheusMetrics = prometheusmetrics.NewMetrics(cfg.Metrics.Prometheus, cfg.Metrics.Disabled)
		engineList = append(engineList, returnEngine.PrometheusMetrics)
	}

	// Now return the proper metrics engine
	if len(engineList) > 1 {
		returnEngine.MetricsEngine = &engineList
	} else if len(engineList) == 1 {
		returnEngine.MetricsEngine = engineList[0]
	} else {
		returnEngine.MetricsEngine = &DummyMetricsEngine{}
	}

	return &returnEngine
}

// DetailedMetricsEngine is a MultiMetricsEngine that preserves links to underlying metrics engines.
type DetailedMetricsEngine struct {
	pbsmetrics.MetricsEngine
	GoMetrics         *pbsmetrics.Metrics
	PrometheusMetrics *prometheusmetrics.Metrics
}

// MultiMetricsEngine logs metrics to multiple metrics databases The can be useful in transitioning
// an instance from one engine to another, you can run both in parallel to verify stats match up.
type MultiMetricsEngine []pbsmetrics.MetricsEngine

// RecordRequest across all engines
func (me *MultiMetricsEngine) RecordRequest(labels pbsmetrics.Labels) {
	for _, thisME := range *me {
		thisME.RecordRequest(labels)
	}
}

func (me *MultiMetricsEngine) RecordConnectionAccept(success bool) {
	for _, thisME := range *me {
		thisME.RecordConnectionAccept(success)
	}
}

func (me *MultiMetricsEngine) RecordConnectionClose(success bool) {
	for _, thisME := range *me {
		thisME.RecordConnectionClose(success)
	}
}

//RecordsImps records imps with imp types across all metric engines
func (me *MultiMetricsEngine) RecordImps(implabels pbsmetrics.ImpLabels) {
	for _, thisME := range *me {
		thisME.RecordImps(implabels)
	}
}

// RecordImps for the legacy endpoint
func (me *MultiMetricsEngine) RecordLegacyImps(labels pbsmetrics.Labels, numImps int) {
	for _, thisME := range *me {
		thisME.RecordLegacyImps(labels, numImps)
	}
}

// RecordRequestTime across all engines
func (me *MultiMetricsEngine) RecordRequestTime(labels pbsmetrics.Labels, length time.Duration) {
	for _, thisME := range *me {
		thisME.RecordRequestTime(labels, length)
	}
}

// RecordAdapterPanic across all engines
func (me *MultiMetricsEngine) RecordAdapterPanic(labels pbsmetrics.AdapterLabels) {
	for _, thisME := range *me {
		thisME.RecordAdapterPanic(labels)
	}
}

// RecordAdapterRequest across all engines
func (me *MultiMetricsEngine) RecordAdapterRequest(labels pbsmetrics.AdapterLabels) {
	for _, thisME := range *me {
		thisME.RecordAdapterRequest(labels)
	}
}

// Keeps track of created and reused connections to adapter bidders and the time from the
// connection request, to the connection creation, or reuse from the pool across all engines
func (me *MultiMetricsEngine) RecordAdapterConnections(bidderName openrtb_ext.BidderName, info httptrace.GotConnInfo, obtainConnectionTime time.Duration) {
	for _, thisME := range *me {
		thisME.RecordAdapterConnections(bidderName, info, obtainConnectionTime)
	}
}

// Times the DNS resolution process
func (me *MultiMetricsEngine) RecordDNSTime(dnsLookupTime time.Duration) {
	for _, thisME := range *me {
		thisME.RecordDNSTime(dnsLookupTime)
	}
}

// RecordAdapterBidReceived across all engines
func (me *MultiMetricsEngine) RecordAdapterBidReceived(labels pbsmetrics.AdapterLabels, bidType openrtb_ext.BidType, hasAdm bool) {
	for _, thisME := range *me {
		thisME.RecordAdapterBidReceived(labels, bidType, hasAdm)
	}
}

// RecordAdapterPrice across all engines
func (me *MultiMetricsEngine) RecordAdapterPrice(labels pbsmetrics.AdapterLabels, cpm float64) {
	for _, thisME := range *me {
		thisME.RecordAdapterPrice(labels, cpm)
	}
}

// RecordAdapterTime across all engines
func (me *MultiMetricsEngine) RecordAdapterTime(labels pbsmetrics.AdapterLabels, length time.Duration) {
	for _, thisME := range *me {
		thisME.RecordAdapterTime(labels, length)
	}
}

// RecordCookieSync across all engines
func (me *MultiMetricsEngine) RecordCookieSync() {
	for _, thisME := range *me {
		thisME.RecordCookieSync()
	}
}

// RecordStoredReqCacheResult across all engines
func (me *MultiMetricsEngine) RecordStoredReqCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
	for _, thisME := range *me {
		thisME.RecordStoredReqCacheResult(cacheResult, inc)
	}
}

// RecordStoredImpCacheResult across all engines
func (me *MultiMetricsEngine) RecordStoredImpCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
	for _, thisME := range *me {
		thisME.RecordStoredImpCacheResult(cacheResult, inc)
	}
}

// RecordAdapterCookieSync across all engines
func (me *MultiMetricsEngine) RecordAdapterCookieSync(adapter openrtb_ext.BidderName, gdprBlocked bool) {
	for _, thisME := range *me {
		thisME.RecordAdapterCookieSync(adapter, gdprBlocked)
	}
}

// RecordUserIDSet across all engines
func (me *MultiMetricsEngine) RecordUserIDSet(userLabels pbsmetrics.UserLabels) {
	for _, thisME := range *me {
		thisME.RecordUserIDSet(userLabels)
	}
}

// RecordPrebidCacheRequestTime across all engines
func (me *MultiMetricsEngine) RecordPrebidCacheRequestTime(success bool, length time.Duration) {
	for _, thisME := range *me {
		thisME.RecordPrebidCacheRequestTime(success, length)
	}
}

// RecordRequestQueueTime across all engines
func (me *MultiMetricsEngine) RecordRequestQueueTime(success bool, requestType pbsmetrics.RequestType, length time.Duration) {
	for _, thisME := range *me {
		thisME.RecordRequestQueueTime(success, requestType, length)
	}
}

// RecordTimeoutNotice across all engines
func (me *MultiMetricsEngine) RecordTimeoutNotice(success bool) {
	for _, thisME := range *me {
		thisME.RecordTimeoutNotice(success)
	}
}

// RecordTCFReq across all engines
func (me *MultiMetricsEngine) RecordTCFReq(version pbsmetrics.TCFVersionValue) {
	for _, thisME := range *me {
		thisME.RecordTCFReq(version)
	}
}

// DummyMetricsEngine is a Noop metrics engine in case no metrics are configured. (may also be useful for tests)
type DummyMetricsEngine struct{}

// RecordRequest as a noop
func (me *DummyMetricsEngine) RecordRequest(labels pbsmetrics.Labels) {
}

// RecordConnectionAccept as a noop
func (me *DummyMetricsEngine) RecordConnectionAccept(success bool) {
}

// RecordConnectionClose as a noop
func (me *DummyMetricsEngine) RecordConnectionClose(success bool) {
}

// RecordImps as a noop
func (me *DummyMetricsEngine) RecordImps(implabels pbsmetrics.ImpLabels) {
}

// RecordLegacyImps as a noop
func (me *DummyMetricsEngine) RecordLegacyImps(labels pbsmetrics.Labels, numImps int) {
}

// RecordRequestTime as a noop
func (me *DummyMetricsEngine) RecordRequestTime(labels pbsmetrics.Labels, length time.Duration) {
}

// RecordAdapterPanic as a noop
func (me *DummyMetricsEngine) RecordAdapterPanic(labels pbsmetrics.AdapterLabels) {
}

// RecordAdapterRequest as a noop
func (me *DummyMetricsEngine) RecordAdapterRequest(labels pbsmetrics.AdapterLabels) {
}

// RecordAdapterConnections as a noop
func (me *DummyMetricsEngine) RecordAdapterConnections(bidderName openrtb_ext.BidderName, info httptrace.GotConnInfo, obtainConnectionTime time.Duration) {
}

// Times the DNS resolution process
func (me *DummyMetricsEngine) RecordDNSTime(dnsLookupTime time.Duration) {
}

// RecordAdapterBidReceived as a noop
func (me *DummyMetricsEngine) RecordAdapterBidReceived(labels pbsmetrics.AdapterLabels, bidType openrtb_ext.BidType, hasAdm bool) {
}

// RecordAdapterPrice as a noop
func (me *DummyMetricsEngine) RecordAdapterPrice(labels pbsmetrics.AdapterLabels, cpm float64) {
}

// RecordAdapterTime as a noop
func (me *DummyMetricsEngine) RecordAdapterTime(labels pbsmetrics.AdapterLabels, length time.Duration) {
}

// RecordCookieSync as a noop
func (me *DummyMetricsEngine) RecordCookieSync() {
}

// RecordAdapterCookieSync as a noop
func (me *DummyMetricsEngine) RecordAdapterCookieSync(adapter openrtb_ext.BidderName, gdprBlocked bool) {
}

// RecordUserIDSet as a noop
func (me *DummyMetricsEngine) RecordUserIDSet(userLabels pbsmetrics.UserLabels) {
}

// RecordStoredReqCacheResult as a noop
func (me *DummyMetricsEngine) RecordStoredReqCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
}

// RecordStoredImpCacheResult as a noop
func (me *DummyMetricsEngine) RecordStoredImpCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
}

// RecordPrebidCacheRequestTime as a noop
func (me *DummyMetricsEngine) RecordPrebidCacheRequestTime(success bool, length time.Duration) {
}

// RecordRequestQueueTime as a noop
func (me *DummyMetricsEngine) RecordRequestQueueTime(success bool, requestType pbsmetrics.RequestType, length time.Duration) {
}

// RecordTimeoutNotice as a noop
func (me *DummyMetricsEngine) RecordTimeoutNotice(success bool) {
}

// RecordReq as a noop
func (me *DummyMetricsEngine) RecordTCFReq(version pbsmetrics.TCFVersionValue) {
}

// MockMetricsEngine will have fields to update via the MetricsEngine interface methods
type MockMetricsData struct {
	DNSLookupTimer float64
}

type MockMetricsEngine func() *MockMetricsData

func NewMockMetricsEngine(metricsData *MockMetricsData) MockMetricsEngine {
	if metricsData == nil {
		metricsData = new(MockMetricsData)
	}
	return func() *MockMetricsData {
		return metricsData
	}
}

// RecordRequest as a noop
func (metricData MockMetricsEngine) RecordRequest(labels pbsmetrics.Labels) {
}

// RecordConnectionAccept as a noop
func (metricData MockMetricsEngine) RecordConnectionAccept(success bool) {
}

// RecordConnectionClose as a noop
func (metricData MockMetricsEngine) RecordConnectionClose(success bool) {
}

// RecordImps as a noop
func (metricData MockMetricsEngine) RecordImps(implabels pbsmetrics.ImpLabels) {
}

// RecordLegacyImps as a noop
func (metricData MockMetricsEngine) RecordLegacyImps(labels pbsmetrics.Labels, numImps int) {
}

// RecordRequestTime as a noop
func (metricData MockMetricsEngine) RecordRequestTime(labels pbsmetrics.Labels, length time.Duration) {
}

// RecordAdapterPanic as a noop
func (metricData MockMetricsEngine) RecordAdapterPanic(labels pbsmetrics.AdapterLabels) {
}

// RecordAdapterRequest as a noop
func (metricData MockMetricsEngine) RecordAdapterRequest(labels pbsmetrics.AdapterLabels) {
}

// RecordAdapterConnections as a noop
func (metricData MockMetricsEngine) RecordAdapterConnections(bidderName openrtb_ext.BidderName, info httptrace.GotConnInfo, obtainConnectionTime time.Duration) {
}

// Times the DNS resolution process
func (metricData MockMetricsEngine) RecordDNSTime(dnsLookupTime time.Duration) {
	metricData().DNSLookupTimer += 1.00
}

// RecordAdapterBidReceived as a noop
func (metricData MockMetricsEngine) RecordAdapterBidReceived(labels pbsmetrics.AdapterLabels, bidType openrtb_ext.BidType, hasAdm bool) {
}

// RecordAdapterPrice as a noop
func (metricData MockMetricsEngine) RecordAdapterPrice(labels pbsmetrics.AdapterLabels, cpm float64) {
}

// RecordAdapterTime as a noop
func (metricData MockMetricsEngine) RecordAdapterTime(labels pbsmetrics.AdapterLabels, length time.Duration) {
}

// RecordCookieSync as a noop
func (metricData MockMetricsEngine) RecordCookieSync() {
}

// RecordAdapterCookieSync as a noop
func (metricData MockMetricsEngine) RecordAdapterCookieSync(adapter openrtb_ext.BidderName, gdprBlocked bool) {
}

// RecordUserIDSet as a noop
func (metricData MockMetricsEngine) RecordUserIDSet(userLabels pbsmetrics.UserLabels) {
}

// RecordStoredReqCacheResult as a noop
func (metricData MockMetricsEngine) RecordStoredReqCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
}

// RecordStoredImpCacheResult as a noop
func (metricData MockMetricsEngine) RecordStoredImpCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
}

// RecordPrebidCacheRequestTime as a noop
func (metricData MockMetricsEngine) RecordPrebidCacheRequestTime(success bool, length time.Duration) {
}

// RecordRequestQueueTime as a noop
func (metricData MockMetricsEngine) RecordRequestQueueTime(success bool, requestType pbsmetrics.RequestType, length time.Duration) {
}

// RecordTimeoutNotice as a noop
func (metricData MockMetricsEngine) RecordTimeoutNotice(success bool) {
}

// RecordReq as a noop
func (metricData MockMetricsEngine) RecordTCFReq(version pbsmetrics.TCFVersionValue) {
}
