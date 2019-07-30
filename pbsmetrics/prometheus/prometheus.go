package prometheusmetrics

import (
	"time"

	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/openrtb_ext"
	"github.com/prebid/prebid-server/pbsmetrics"
	"github.com/prometheus/client_golang/prometheus"
	_ "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Defines the actual Prometheus metrics we will be using. Satisfies interface MetricsEngine
type Metrics struct {
	Registry             *prometheus.Registry
	connCounter          prometheus.Gauge
	connError            *prometheus.CounterVec
	imps                 *prometheus.CounterVec
	legacyImps           *prometheus.CounterVec
	requests             *prometheus.CounterVec
	reqTimer             *prometheus.HistogramVec
	adaptRequests        *prometheus.CounterVec
	adaptTimer           *prometheus.HistogramVec
	adaptBids            *prometheus.CounterVec
	adaptPrices          *prometheus.HistogramVec
	adaptErrors          *prometheus.CounterVec
	adaptPanics          *prometheus.CounterVec
	cookieSync           prometheus.Counter
	adaptCookieSync      *prometheus.CounterVec
	userID               *prometheus.CounterVec
	storedReqCacheResult *prometheus.CounterVec
	storedImpCacheResult *prometheus.CounterVec
}

const (
	requestTypeLabel    = "request_type"
	demandSourceLabel   = "demand_source"
	browserLabel        = "browser"
	cookieLabel         = "cookie"
	responseStatusLabel = "response_status"
	adapterLabel        = "adapter"
	adapterBidLabel     = "adapter_bid"
	markupTypeLabel     = "markup_type"
	bidTypeLabel        = "bid_type"
	adapterErrLabel     = "adapter_error"
	cacheResultLabel    = "cache_result"
	gdprBlockedLabel    = "gdpr_blocked"
	bannerLabel         = "banner"
	videoLabel          = "video"
	audioLabel          = "audio"
	nativeLabel         = "native"
)

// NewMetrics constructs the appropriate options for the Prometheus metrics. Needs to be fed the promethus config
// Its own function to keep the metric creation function cleaner.
func NewMetrics(cfg config.PrometheusMetrics) *Metrics {
	// define the buckets for timers
	timerBuckets := prometheus.LinearBuckets(0.05, 0.05, 20)
	timerBuckets = append(timerBuckets, []float64{1.5, 2.0, 3.0, 5.0, 20.0, 50.0}...)

	standardLabelNames := []string{demandSourceLabel, requestTypeLabel, browserLabel, cookieLabel, responseStatusLabel}

	adapterLabelNames := []string{demandSourceLabel, requestTypeLabel, browserLabel, cookieLabel, adapterBidLabel, adapterLabel}
	bidLabelNames := []string{demandSourceLabel, requestTypeLabel, browserLabel, cookieLabel, adapterBidLabel, adapterLabel, bidTypeLabel, markupTypeLabel}
	errorLabelNames := []string{demandSourceLabel, requestTypeLabel, browserLabel, cookieLabel, adapterErrLabel, adapterLabel}

	impLabelNames := []string{bannerLabel, videoLabel, audioLabel, nativeLabel}

	metrics := Metrics{}
	metrics.Registry = prometheus.NewRegistry()
	metrics.connCounter = newConnCounter(cfg)
	metrics.Registry.MustRegister(metrics.connCounter)
	metrics.connError = newCounter(cfg, "active_connections_total",
		"Errors reported on the connections coming in.",
		[]string{"ErrorType"},
	)
	metrics.Registry.MustRegister(metrics.connError)
	metrics.imps = newCounter(cfg, "imps_requested",
		"Count of Impressions by type and in total requested through PBS.",
		impLabelNames,
	)
	metrics.Registry.MustRegister(metrics.imps)
	metrics.legacyImps = newCounter(cfg, "legacy_imps_requested",
		"Total number of impressions requested through legacy PBS.",
		standardLabelNames,
	)
	metrics.Registry.MustRegister(metrics.legacyImps)
	metrics.requests = newCounter(cfg, "requests_total",
		"Total number of requests made to PBS.",
		standardLabelNames,
	)
	metrics.Registry.MustRegister(metrics.requests)
	metrics.reqTimer = newHistogram(cfg, "request_time_seconds",
		"Seconds to resolve each PBS request.",
		standardLabelNames, timerBuckets,
	)
	metrics.Registry.MustRegister(metrics.reqTimer)
	metrics.adaptRequests = newCounter(cfg, "adapter_requests_total",
		"Number of requests sent out to each bidder.",
		adapterLabelNames,
	)
	metrics.Registry.MustRegister(metrics.adaptRequests)
	metrics.adaptPanics = newCounter(cfg, "adapter_panics_total",
		"Number of panics generated by each bidder.",
		adapterLabelNames,
	)
	metrics.Registry.MustRegister(metrics.adaptPanics)
	metrics.adaptTimer = newHistogram(cfg, "adapter_time_seconds",
		"Seconds to resolve each request to a bidder.",
		adapterLabelNames, timerBuckets,
	)
	metrics.Registry.MustRegister(metrics.adaptTimer)
	metrics.adaptBids = newCounter(cfg, "adapter_bids_received_total",
		"Number of bids received from each bidder.",
		bidLabelNames,
	)
	metrics.Registry.MustRegister(metrics.adaptBids)
	metrics.storedReqCacheResult = newCounter(cfg, "stored_request_cache_performance",
		"Number of stored request cache hits vs miss",
		[]string{"cache_result"},
	)
	metrics.Registry.MustRegister(metrics.storedReqCacheResult)
	metrics.storedImpCacheResult = newCounter(cfg, "stored_imp_cache_performance",
		"Number of stored imp cache hits vs miss",
		[]string{"cache_result"},
	)
	metrics.Registry.MustRegister(metrics.storedImpCacheResult)
	metrics.adaptPrices = newHistogram(cfg, "adapter_prices",
		"Values of the bids from each bidder.",
		adapterLabelNames, prometheus.LinearBuckets(0.1, 0.1, 200),
	)
	metrics.Registry.MustRegister(metrics.adaptPrices)
	metrics.adaptErrors = newCounter(cfg, "adapter_errors_total",
		"Number of unique error types seen in each request to an adapter.",
		errorLabelNames,
	)
	metrics.Registry.MustRegister(metrics.adaptErrors)
	metrics.cookieSync = newCookieSync(cfg)
	metrics.Registry.MustRegister(metrics.cookieSync)
	metrics.adaptCookieSync = newCounter(cfg, "cookie_sync_returns",
		"Number of syncs generated for a bidder, and if they were subsequently blocked.",
		[]string{adapterLabel, gdprBlockedLabel},
	)
	metrics.Registry.MustRegister(metrics.adaptCookieSync)
	metrics.userID = newCounter(cfg, "setuid_calls",
		"Number of user ID syncs performed",
		[]string{"action", "bidder"},
	)
	metrics.Registry.MustRegister(metrics.userID)

	initializeTimeSeries(&metrics)

	return &metrics
}

func newConnCounter(cfg config.PrometheusMetrics) prometheus.Gauge {
	opts := prometheus.GaugeOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      "active_connections",
		Help:      "Current number of active (open) connections.",
	}
	return prometheus.NewGauge(opts)
}

func newCookieSync(cfg config.PrometheusMetrics) prometheus.Counter {
	opts := prometheus.CounterOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      "cookie_sync_requests_total",
		Help:      "Number of cookie sync requests received.",
	}
	return prometheus.NewCounter(opts)
}

func newCounter(cfg config.PrometheusMetrics, name string, help string, labels []string) *prometheus.CounterVec {
	opts := prometheus.CounterOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      name,
		Help:      help,
	}
	return prometheus.NewCounterVec(opts, labels)
}

func newHistogram(cfg config.PrometheusMetrics, name string, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	opts := prometheus.HistogramOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      name,
		Help:      help,
		Buckets:   buckets,
	}
	return prometheus.NewHistogramVec(opts, labels)
}

func (me *Metrics) RecordConnectionAccept(success bool) {
	if success {
		me.connCounter.Inc()
	} else {
		me.connError.WithLabelValues("accept_error").Inc()
	}

}

func (me *Metrics) RecordConnectionClose(success bool) {
	if success {
		me.connCounter.Dec()
	} else {
		me.connError.WithLabelValues("close_error").Inc()
	}
}

func (me *Metrics) RecordRequest(labels pbsmetrics.Labels) {
	me.requests.With(resolveLabels(labels)).Inc()
}

func (me *Metrics) RecordImps(implabels pbsmetrics.ImpLabels) {
	me.imps.With(resolveImpLabels(implabels)).Inc()
}

func (me *Metrics) RecordLegacyImps(labels pbsmetrics.Labels, numImps int) {
	var lbls prometheus.Labels
	lbls = resolveLabels(labels)
	me.legacyImps.With(lbls).Add(float64(numImps))
}

func (me *Metrics) RecordRequestTime(labels pbsmetrics.Labels, length time.Duration) {
	time := float64(length) / float64(time.Second)
	me.reqTimer.With(resolveLabels(labels)).Observe(time)
}

func (me *Metrics) RecordAdapterPanic(labels pbsmetrics.AdapterLabels) {
	me.adaptPanics.With(resolveAdapterLabels(labels)).Inc()
}

func (me *Metrics) RecordAdapterRequest(labels pbsmetrics.AdapterLabels) {
	me.adaptRequests.With(resolveAdapterLabels(labels)).Inc()
	for k := range labels.AdapterErrors {
		me.adaptErrors.With(resolveAdapterErrorLabels(labels, string(k))).Inc()
	}
}

func (me *Metrics) RecordAdapterBidReceived(labels pbsmetrics.AdapterLabels, bidType openrtb_ext.BidType, hasAdm bool) {
	me.adaptBids.With(resolveBidLabels(labels, bidType, hasAdm)).Inc()
}

func (me *Metrics) RecordAdapterPrice(labels pbsmetrics.AdapterLabels, cpm float64) {
	me.adaptPrices.With(resolveAdapterLabels(labels)).Observe(cpm)
}

func (me *Metrics) RecordAdapterTime(labels pbsmetrics.AdapterLabels, length time.Duration) {
	time := float64(length) / float64(time.Second)
	me.adaptTimer.With(resolveAdapterLabels(labels)).Observe(time)
}

func (me *Metrics) RecordCookieSync(labels pbsmetrics.Labels) {
	me.cookieSync.Inc()
}

func (me *Metrics) RecordAdapterCookieSync(adapter openrtb_ext.BidderName, gdprBlocked bool) {
	labels := prometheus.Labels{
		adapterLabel: string(adapter),
	}
	if gdprBlocked {
		labels[gdprBlockedLabel] = "true"
	} else {
		labels[gdprBlockedLabel] = "false"
	}
	me.adaptCookieSync.With(labels).Inc()
}

// RecordStoredReqCacheResult records cache hits and misses when looking up stored requests
func (me *Metrics) RecordStoredReqCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
	labels := prometheus.Labels{
		cacheResultLabel: string(cacheResult),
	}

	me.storedReqCacheResult.With(labels).Add(float64(inc))
}

// RecordStoredImpCacheResult records cache hits and misses when looking up stored imps
func (me *Metrics) RecordStoredImpCacheResult(cacheResult pbsmetrics.CacheResult, inc int) {
	labels := prometheus.Labels{
		cacheResultLabel: string(cacheResult),
	}

	me.storedImpCacheResult.With(labels).Add(float64(inc))
}

func (me *Metrics) RecordUserIDSet(userLabels pbsmetrics.UserLabels) {
	me.userID.With(resolveUserSyncLabels(userLabels)).Inc()
}

func resolveLabels(labels pbsmetrics.Labels) prometheus.Labels {
	return prometheus.Labels{
		demandSourceLabel: string(labels.Source),
		requestTypeLabel:  string(labels.RType),
		// "pubid":   labels.PubID,
		browserLabel:        string(labels.Browser),
		cookieLabel:         string(labels.CookieFlag),
		responseStatusLabel: string(labels.RequestStatus),
	}
}

func resolveAdapterLabels(labels pbsmetrics.AdapterLabels) prometheus.Labels {
	return prometheus.Labels{
		demandSourceLabel: string(labels.Source),
		requestTypeLabel:  string(labels.RType),
		// "pubid":   labels.PubID,
		browserLabel:    string(labels.Browser),
		cookieLabel:     string(labels.CookieFlag),
		adapterBidLabel: string(labels.AdapterBids),
		adapterLabel:    string(labels.Adapter),
	}
}

func resolveBidLabels(labels pbsmetrics.AdapterLabels, bidType openrtb_ext.BidType, hasAdm bool) prometheus.Labels {
	bidLabels := prometheus.Labels{
		demandSourceLabel: string(labels.Source),
		requestTypeLabel:  string(labels.RType),
		// "pubid":   labels.PubID,
		browserLabel:    string(labels.Browser),
		cookieLabel:     string(labels.CookieFlag),
		adapterBidLabel: string(labels.AdapterBids),
		adapterLabel:    string(labels.Adapter),
		bidTypeLabel:    string(bidType),
		markupTypeLabel: "unknown",
	}
	if hasAdm {
		bidLabels[markupTypeLabel] = "adm"
	}
	return bidLabels
}

func resolveAdapterErrorLabels(labels pbsmetrics.AdapterLabels, errorType string) prometheus.Labels {
	return prometheus.Labels{
		demandSourceLabel: string(labels.Source),
		requestTypeLabel:  string(labels.RType),
		// "pubid":   labels.PubID,
		browserLabel:    string(labels.Browser),
		cookieLabel:     string(labels.CookieFlag),
		adapterErrLabel: errorType,
		adapterLabel:    string(labels.Adapter),
	}
}

func resolveUserSyncLabels(userLabels pbsmetrics.UserLabels) prometheus.Labels {
	return prometheus.Labels{
		"action": string(userLabels.Action),
		"bidder": string(userLabels.Bidder),
	}
}

func resolveImpLabels(labels pbsmetrics.ImpLabels) prometheus.Labels {
	var impLabels prometheus.Labels = prometheus.Labels{
		bannerLabel: "no",
		videoLabel:  "no",
		audioLabel:  "no",
		nativeLabel: "no",
	}
	if labels.BannerImps {
		impLabels[bannerLabel] = "yes"
	}
	if labels.VideoImps {
		impLabels[videoLabel] = "yes"
	}
	if labels.AudioImps {
		impLabels[audioLabel] = "yes"
	}
	if labels.NativeImps {
		impLabels[nativeLabel] = "yes"
	}
	return impLabels
}

// initializeTimeSeries precreates all possible metric label values, so there is no locking needed at run time creating new instances
func initializeTimeSeries(m *Metrics) {
	// Connection errors
	labels := addDimension([]prometheus.Labels{}, "ErrorType", []string{"accept_error", "close_error"})
	for _, l := range labels {
		_ = m.connError.With(l)
	}

	// Standard labels
	labels = addDimension([]prometheus.Labels{}, demandSourceLabel, demandTypesAsString())
	labels = addDimension(labels, requestTypeLabel, requestTypesAsString())
	labels = addDimension(labels, browserLabel, browserTypesAsString())
	labels = addDimension(labels, cookieLabel, cookieTypesAsString())
	adapterLabels := labels // save regenerating these dimensions for adapter status
	labels = addDimension(labels, responseStatusLabel, requestStatusesAsString())
	for _, l := range labels {
		_ = m.requests.With(l)
		_ = m.reqTimer.With(l)
	}

	// Adapter labels
	labels = addDimension(adapterLabels, adapterLabel, adaptersAsString())
	errorLabels := labels // save regenerating these dimensions for adapter errors
	labels = addDimension(labels, adapterBidLabel, adapterBidsAsString())
	for _, l := range labels {
		_ = m.adaptRequests.With(l)
		_ = m.adaptTimer.With(l)
		_ = m.adaptPrices.With(l)
		_ = m.adaptPanics.With(l)
	}
	// AdapterBid labels
	labels = addDimension(labels, bidTypeLabel, bidTypesAsString())
	labels = addDimension(labels, markupTypeLabel, []string{"unknown", "adm"})
	for _, l := range labels {
		_ = m.adaptBids.With(l)
	}
	labels = addDimension(errorLabels, adapterErrLabel, adapterErrorsAsString())
	for _, l := range labels {
		_ = m.adaptErrors.With(l)
	}
	cookieLabels := addDimension([]prometheus.Labels{}, adapterLabel, adaptersAsString())
	cookieLabels = addDimension(cookieLabels, gdprBlockedLabel, []string{"true", "false"})
	for _, l := range cookieLabels {
		_ = m.adaptCookieSync.With(l)
	}
	cacheLabels := addDimension([]prometheus.Labels{}, "cache_result", cacheResultAsString())
	for _, l := range cacheLabels {
		_ = m.storedImpCacheResult.With(l)
		_ = m.storedReqCacheResult.With(l)
	}

	// ImpType labels
	impTypeLabels := addDimension([]prometheus.Labels{}, bannerLabel, []string{"yes", "no"})
	impTypeLabels = addDimension(impTypeLabels, videoLabel, []string{"yes", "no"})
	impTypeLabels = addDimension(impTypeLabels, audioLabel, []string{"yes", "no"})
	impTypeLabels = addDimension(impTypeLabels, nativeLabel, []string{"yes", "no"})
	for _, l := range impTypeLabels {
		_ = m.imps.With(l)
	}
}

// addDimesion will expand a slice of labels to add the dimension of a new set of values for a new label name
func addDimension(labels []prometheus.Labels, field string, values []string) []prometheus.Labels {
	if len(labels) == 0 {
		// We are starting a new slice of labels, so we can't loop.
		return addToLabel(make(prometheus.Labels), field, values)
	}
	newLabels := make([]prometheus.Labels, 0, len(labels)*len(values))
	for _, l := range labels {
		newLabels = append(newLabels, addToLabel(l, field, values)...)
	}
	return newLabels
}

// addToLabel will create a slice of labels adding a set of values tied to a label name.
func addToLabel(label prometheus.Labels, field string, values []string) []prometheus.Labels {
	newLabels := make([]prometheus.Labels, len(values))
	for i, v := range values {
		l := copyLabel(label)
		l[field] = v
		newLabels[i] = l
	}
	return newLabels
}

// Need to be able to deep copy prometheus labels.
func copyLabel(label prometheus.Labels) prometheus.Labels {
	newLabel := make(prometheus.Labels)
	for k, v := range label {
		newLabel[k] = v
	}
	return newLabel
}

func demandTypesAsString() []string {
	list := pbsmetrics.DemandTypes()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func requestTypesAsString() []string {
	list := pbsmetrics.RequestTypes()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func browserTypesAsString() []string {
	list := pbsmetrics.BrowserTypes()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func cookieTypesAsString() []string {
	list := pbsmetrics.CookieTypes()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func requestStatusesAsString() []string {
	list := pbsmetrics.RequestStatuses()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func adapterBidsAsString() []string {
	list := pbsmetrics.AdapterBids()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func adapterErrorsAsString() []string {
	list := pbsmetrics.AdapterErrors()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func cacheResultAsString() []string {
	list := pbsmetrics.CacheResults()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output
}

func adaptersAsString() []string {
	list := openrtb_ext.BidderList()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output

}

func bidTypesAsString() []string {
	list := openrtb_ext.BidTypes()
	output := make([]string, len(list))
	for i, s := range list {
		output[i] = string(s)
	}
	return output

}
