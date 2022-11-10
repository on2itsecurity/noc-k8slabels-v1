package panosapi

import (
	"crypto/tls"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/url"
	"noc-k8slabels-v1/container/go/pkg/config"
	"strings"
	"time"

	"github.com/charithe/timedbuf"
	"github.com/sirupsen/logrus"
)

var c = config.Load()
var tbUpdate = timedbuf.New(500, 2*time.Second, flushUpdateBuffer)
var tbRemove = timedbuf.New(500, 20*time.Second, flushRemoveBuffer)

func httpClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     90 * time.Second, // Palo Alto has a keepalive of 90 seconds server side, so lets do this also client side
		},
		Timeout: 5 * time.Second,
	}

	return client
}

var client = httpClient()

type uidMessage struct {
	XMLName xml.Name  `xml:"uid-message"`
	Type    string    `xml:"type"`
	Payload []payload `xml:"payload"`
}

type payload struct {
	Clear      *clear      `xml:"clear"`
	UnRegister *unRegister `xml:"unregister"`
	Register   *register   `xml:"register"`
}

type clear struct {
	RegisteredIP registeredIP `xml:"registered-ip"`
}

type registeredIP struct {
	All string `xml:"all"`
}

type register struct {
	Entry []entry `xml:"entry"`
}
type unRegister struct {
	Entry []entry `xml:"entry"`
}

type entry struct {
	IP         net.IP `xml:"ip,attr"`
	Persistent int    `xml:"persistent,attr,omitempty"`
	Tag        tag    `xml:"tag"`
}

type tag struct {
	Member []member `xml:"member"`
}

type member struct {
	Text    string `xml:",chardata"`
	Timeout int    `xml:"timeout,attr,omitempty"`
}

type ipLabels struct {
	IP     net.IP
	Labels []string
}

func sendUpdatePanAPIs(requestBody string) {
	for _, panfwurl := range c.PanFW.URL {
		counter.panTotalAPICalls++
		address := panfwurl + "/api"
		sendUpdatePanAPI(requestBody, address)
	}
}

func sendUpdatePanAPI(requestBody string, address string) {
	data := url.Values{}
	data.Set("type", "user-id")
	data.Add("key", c.PanFW.Token)
	data.Add("cmd", requestBody)

	req, _ := http.NewRequest("POST", address, strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)

	if resp == nil {
		counter.panTotalFailedAPICalls++
		counter.panFailedAPICalls++
		logrus.WithField("pkg", "panapi").Errorf("Could not complete http(s) call to PAN-FW XML-API %s", address)
		return
	}
	defer resp.Body.Close()

	if err != nil {
		counter.panTotalFailedAPICalls++
		counter.panFailedAPICalls++
		logrus.WithField("pkg", "panapi").Errorf("response from pan xml api %s: %s", address, err)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		counter.panTotalFailedAPICalls++
		counter.panResponseParsingError++
		logrus.WithField("pkg", "panapi").Errorf("error reading response body from %s: %s", address, err)
		return
	}

	if resp.StatusCode > 299 {
		counter.panTotalFailedAPICalls++
		counter.panResponseHTTPCodeError++
		logrus.WithField("pkg", "panapi").Errorf("response from pan xml api %s: %s", address, body)

	}
	counter.panSuccessAPICalls++
}

func labelsToMemberSlice(labels []string) []member {
	var tagMembers []member

	for _, label := range labels {
		tagMember := member{Text: label, Timeout: c.PanFW.RegisterExpire}
		tagMembers = append(tagMembers, tagMember)
	}
	return tagMembers
}

func ipLabelsToSlice(ipK8Slabels []ipLabels) []entry {
	var regEntries []entry

	for _, ipK8Slabel := range ipK8Slabels {
		regEntry := entry{ipK8Slabel.IP, 1, tag{labelsToMemberSlice(ipK8Slabel.Labels)}}
		regEntries = append(regEntries, regEntry)
	}
	return regEntries
}

// unregisterRegEntriesSlice removes unneeded/unwanted persistent and timeout attributes from the slice
func unregisterRegEntriesSlice(regEntries []entry) []entry {
	var unregEntries []entry

	copy(unregEntries, regEntries)

	for numEntry := range unregEntries {
		unregEntries[numEntry].Persistent = 0
		for numMember := range unregEntries[numEntry].Tag.Member {
			unregEntries[numEntry].Tag.Member[numMember].Timeout = 0
		}
	}
	return unregEntries
}

// Fully update the ip: as per http://api-lab.paloaltonetworks.com/registered-ip.html:
// "When register and unregister are combined in a single document, the entries are processed in the order: unregister, register; only a single <register/> and <unregister/> section should be specified."
func generateUpdateSlice(regEntries []entry) *uidMessage {
	body := &uidMessage{
		Type: "update",
		Payload: []payload{
			{
				Register: &register{
					regEntries,
				},
				UnRegister: &unRegister{
					unregisterRegEntriesSlice(regEntries),
				},
			},
		},
	}
	return body
}
func generateRemoveSlice(regEntries []entry) *uidMessage {
	body := &uidMessage{
		Type: "update",
		Payload: []payload{
			{
				UnRegister: &unRegister{
					unregisterRegEntriesSlice(regEntries),
				},
			},
		},
	}
	return body
}

func generateUpdateXML(body *uidMessage) string {
	requestBody, err := xml.Marshal(body)
	if err != nil {
		logrus.WithField("pkg", "panapi").Errorf("error: %v", err)
		return ""
	}
	return string(requestBody)
}

func flushUpdateBuffer(items []interface{}) {
	var ipItems []ipLabels

	for _, item := range items {
		ipItems = append(ipItems, item.(ipLabels))
		counter.panUpdatedIps++
	}
	ipLabelItems := generateUpdateSlice(ipLabelsToSlice(ipItems))
	requestBody := generateUpdateXML(ipLabelItems)

	sendUpdatePanAPIs(requestBody)
	logrus.WithField("pkg", "panapi").Debugf("Request body send to PaloAlto: %s", requestBody)
}

func flushRemoveBuffer(items []interface{}) {
	var ipItems []ipLabels

	for _, item := range items {
		ipItems = append(ipItems, item.(ipLabels))
		counter.panRemovedIps++
	}
	ipLabelItems := generateRemoveSlice(ipLabelsToSlice(ipItems))
	requestBody := generateUpdateXML(ipLabelItems)

	sendUpdatePanAPIs(requestBody)
	logrus.WithField("pkg", "panapi").Debugf("Request body send to PaloAlto: %s", requestBody)
}

// Shutdown flushes and closes the buffer
func Shutdown() {
	tbUpdate.Close()
	tbRemove.Close()
}

// BatchUpdateIP sends one ip update to the Palo Alto Firewall
func BatchUpdateIPs(ip net.IP, labels string) {
	if !strings.Contains(labels, "=") {
		return
	}
	tbUpdate.Put(ipLabels{ip, strings.Split(labels, ",")})
}

// BatchRemoveIPs sends one ip update to the Palo Alto Firewall
func BatchRemoveIPs(ip net.IP, labels string) {
	tbRemove.Put(ipLabels{ip, strings.Split(labels, ",")})
}

// UpdateOneIP sends one ip update to the Palo Alto Firewall
func UpdateOneIP(ip net.IP, labels string) {
	if !strings.Contains(labels, "=") {
		return
	}
	requestBodySlice := generateUpdateSlice(
		ipLabelsToSlice(
			[]ipLabels{
				{IP: ip, Labels: strings.Split(labels, ",")},
			},
		),
	)

	requestBody := generateUpdateXML(requestBodySlice)
	counter.panUpdatedIps++
	sendUpdatePanAPIs(requestBody)
}

// RemoveOneIP removes one registration from the Palo Alto Firewall
func RemoveOneIP(ip net.IP, labels string) {
	requestBodySlice := generateRemoveSlice(
		ipLabelsToSlice(
			[]ipLabels{
				{IP: ip, Labels: strings.Split(labels, ",")},
			},
		),
	)

	requestBody := generateUpdateXML(requestBodySlice)
	counter.panRemovedIps++
	sendUpdatePanAPIs(requestBody)
}
