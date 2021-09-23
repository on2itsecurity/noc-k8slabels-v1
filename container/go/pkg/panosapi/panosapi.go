package panosapi

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"noc-k8slabels-v1/container/go/pkg/config"
	"os"
	"strings"
	"time"
)

var c = config.Load()

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
	Persistent int    `xml:"persistent,attr"`
	Tag        tag    `xml:"tag"`
}

type tag struct {
	Member []member `xml:"member"`
}

type member struct {
	Text    string `xml:",chardata"`
	Timeout int    `xml:"timeout,attr"`
}

type ipLabels struct {
	IP     net.IP
	Labels []string
}

func sendUpdatePanAPIs(requestBody string) {
	for _, panfwurl := range c.PanFW.URL {
		address := panfwurl + "/api"
		sendUpdatePanAPI(requestBody, address)
	}
}

func sendUpdatePanAPI(requestBody string, address string) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   1 * time.Second,
	}

	data := url.Values{}
	data.Set("type", "user-id")
	data.Add("key", c.PanFW.Token)
	data.Add("cmd", requestBody)

	req, _ := http.NewRequest("POST", address, strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)

	if resp == nil {
		fmt.Printf("Could not complete http(s) call to PAN-FW XML-API %s\n", address)
		return
	}

	if err != nil {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("response from pan xml api %s: %s\n", address, body)
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("response from pan xml api %s: %s\n", address, body)

	}
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

// Fully update the ip: as per http://api-lab.paloaltonetworks.com/registered-ip.html:
// "When register and unregister are combined in a single document, the entries are processed in the order: unregister, register; only a single <register/> and <unregister/> section should be specified."
func generateUpdateSlice(regEntries []entry) *uidMessage {
	body := &uidMessage{
		Type: "update",
		Payload: []payload{
			payload{
				Register: &register{
					regEntries,
				},
				UnRegister: &unRegister{
					regEntries,
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
			payload{
				UnRegister: &unRegister{
					regEntries,
				},
			},
		},
	}
	return body
}

func generateUpdateXML(body *uidMessage) string {
	requestBody, err := xml.Marshal(body)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	return string(requestBody)
}

// UpdateOneIP sends one ip update to the Palo Alto Firewall
func UpdateOneIP(ip net.IP, labels string) {
	if !strings.Contains(labels, "=") {
		return
	}
	requestBodySlice := generateUpdateSlice(
		ipLabelsToSlice(
			[]ipLabels{
				ipLabels{IP: ip, Labels: strings.Split(labels, ",")}}))

	requestBody := generateUpdateXML(requestBodySlice)

	//fmt.Printf("send to pan api xml: %s\n", requestBody)
	sendUpdatePanAPIs(requestBody)
}

// RemoveOneIP removes one registration from the Palo Alto Firewall
func RemoveOneIP(ip net.IP, labels string) {
	requestBodySlice := generateRemoveSlice(
		ipLabelsToSlice(
			[]ipLabels{
				ipLabels{IP: ip, Labels: strings.Split(labels, ",")}}))

	requestBody := generateUpdateXML(requestBodySlice)

	//fmt.Printf("send to pan api xml: %s\n", requestBody)
	sendUpdatePanAPIs(requestBody)
}

// ClearAll clears all registrations everything
func ClearAll() {

	requestBodySlice := &uidMessage{Type: "update", Payload: []payload{payload{Clear: &clear{}}}}
	requestBody := generateUpdateXML(requestBodySlice)

	//fmt.Printf("send to pan api xml: %s\n", requestBody)
	sendUpdatePanAPIs(requestBody)
}

// ReplaceRegisterAll Clears all registrations from the Palo Alto Firewall and registers all ip/labels at once.
func ReplaceRegisterAll(ipK8Slabels []ipLabels) {
	requestBodySlice := generateUpdateSlice(
		ipLabelsToSlice(
			ipK8Slabels,
		))

	payloadClear := payload{Clear: &clear{}}
	requestBodySlice.Payload = append(requestBodySlice.Payload, payloadClear)
	requestBody := generateUpdateXML(requestBodySlice)

	//fmt.Printf("send to pan api xml: %s", requestBody)
	sendUpdatePanAPIs(requestBody)
}
