package panosapi

import "github.com/prometheus/client_golang/prometheus"

type panCounter struct {
	panTotalAPICalls         int
	panSuccessAPICalls       int
	panUpdatedIps            int
	panRemovedIps            int
	panTotalFailedAPICalls   int
	panFailedAPICalls        int
	panResponseParsingError  int
	panResponseHTTPCodeError int
}

type panCollector struct {
	panTotalAPICalls         *prometheus.Desc
	panSuccessAPICalls       *prometheus.Desc
	panUpdatedIps            *prometheus.Desc
	panRemovedIps            *prometheus.Desc
	panTotalFailedAPICalls   *prometheus.Desc
	panFailedAPICalls        *prometheus.Desc
	panResponseParsingError  *prometheus.Desc
	panResponseHTTPCodeError *prometheus.Desc
}

var namespace string = "k8slabels"
var substring string = "panosapi"

var counter panCounter = panCounter{0, 0, 0, 0, 0, 0, 0, 0}

func newPanCollector() *panCollector {
	return &panCollector{
		panTotalAPICalls: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "apicalls_count"),
			"Total api calls to firewall",
			[]string{"name", "path"}, nil,
		),
		panSuccessAPICalls: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "apicalls_success_count"),
			"Sucessfull api calls to firewall",
			[]string{"name", "path"}, nil,
		),
		panUpdatedIps: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "updated_ips_count"),
			"Updated pod ips to firewall",
			[]string{"name", "path"}, nil,
		),
		panRemovedIps: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "removed_ips_count"),
			"Removed pod ips from firewall",
			[]string{"name", "path"}, nil,
		),
		panTotalFailedAPICalls: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "apicalls_failed_count"),
			"Total failed api calls to firewall",
			[]string{"name", "path"}, nil,
		),
		panFailedAPICalls: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "apicalls_failed_count"),
			"Failed api calls to firewall",
			[]string{"name", "path"}, nil,
		),
		panResponseParsingError: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "response_parsing_error_count"),
			"Response from firewall parsing error",
			[]string{"name", "path"}, nil,
		),
		panResponseHTTPCodeError: prometheus.NewDesc(prometheus.BuildFQName(namespace, substring, "response_http_error_count"),
			"Error Response from firewall",
			[]string{"name", "path"}, nil,
		),
	}
}

// Each and every collector must implement the Describe function.
// It essentially writes all descriptors to the prometheus desc channel.
func (collector *panCollector) Describe(ch chan<- *prometheus.Desc) {

	//Update this section with the each metric you create for a given collector
	ch <- collector.panTotalAPICalls
	ch <- collector.panSuccessAPICalls
	ch <- collector.panUpdatedIps
	ch <- collector.panRemovedIps
	ch <- collector.panTotalFailedAPICalls
	ch <- collector.panFailedAPICalls
	ch <- collector.panResponseParsingError
	ch <- collector.panResponseHTTPCodeError

}

// Collect implements required collect function for all promehteus collectors
func (collector *panCollector) Collect(ch chan<- prometheus.Metric) {

	//Write latest value for each metric in the prometheus metric channel.
	//Note that you can pass CounterValue, GaugeValue, or UntypedValue types here.
	ch <- prometheus.MustNewConstMetric(collector.panTotalAPICalls, prometheus.CounterValue, float64(counter.panTotalAPICalls), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panSuccessAPICalls, prometheus.CounterValue, float64(counter.panSuccessAPICalls), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panUpdatedIps, prometheus.CounterValue, float64(counter.panUpdatedIps), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panRemovedIps, prometheus.CounterValue, float64(counter.panRemovedIps), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panTotalFailedAPICalls, prometheus.CounterValue, float64(counter.panTotalFailedAPICalls), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panFailedAPICalls, prometheus.CounterValue, float64(counter.panFailedAPICalls), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panResponseParsingError, prometheus.CounterValue, float64(counter.panResponseParsingError), "log", "path")
	ch <- prometheus.MustNewConstMetric(collector.panResponseHTTPCodeError, prometheus.CounterValue, float64(counter.panResponseHTTPCodeError), "log", "path")
	counter = panCounter{0, 0, 0, 0, 0, 0, 0, 0}

}
func Register() {
	collector := newPanCollector()
	prometheus.MustRegister(collector)
}
